package inserter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/codeuniversity/smag-mvp/models"
	"github.com/codeuniversity/smag-mvp/service"
	bolt "github.com/johnnadratowski/golang-neo4j-bolt-driver"

	"github.com/segmentio/kafka-go"
)

// Inserter represents the scraper containing all clients it uses
type Inserter struct {
	qReader *kafka.Reader
	qWriter *kafka.Writer
	driver  bolt.Driver
	conn    bolt.Conn
	//result  neo4j.Result
	*service.Executor
}

// New returns an initilized scraper
func New(kafkaAddress, neo4jAddress string) *Inserter {
	i := &Inserter{}
	i.qReader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaAddress},
		GroupID:        "user_follow_inserter",
		Topic:          "user_follow_infos",
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		CommitInterval: time.Second,
	})
	i.qWriter = kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{kafkaAddress},
		Topic:    "user_names",
		Balancer: &kafka.LeastBytes{},
		Async:    true,
	})
	driver := bolt.NewDriver()
	con, err := driver.OpenNeo(neo4jAddress)
	if err != nil {
		panic(err)
	}
	i.conn = con
	i.Executor = service.New()
	return i
}

// Run the inserter
func (i *Inserter) Run() {
	defer func() {
		i.MarkAsStopped()
	}()
	fmt.Println("starting inserter")
	for i.IsRunning() {
		m, err := i.qReader.FetchMessage(context.Background())
		if err != nil {
			fmt.Println(err)
			break
		}
		info := &models.UserFollowInfo{}

		err = json.Unmarshal(m.Value, info)
		if err != nil {
			fmt.Println(err)
			break
		}
		fmt.Println("inserting: ", info.UserName)
		i.InsertUserFollowInfo(info)
		i.qReader.CommitMessages(context.Background(), m)
		fmt.Println("commited: ", info.UserName)

	}

}

// InsertUserFollowInfo inserts the user follow info into dgraph, while writting userNames that don't exist in the graph yet
// into the specified kafka topic
func (i *Inserter) InsertUserFollowInfo(followInfo *models.UserFollowInfo) {
	p := &models.User{
		Name:      followInfo.UserName,
		RealName:  followInfo.RealName,
		AvatarURL: followInfo.AvatarURL,
		Bio:       followInfo.Bio,
		CrawledAt: followInfo.CrawlTs,
	}

	for _, following := range followInfo.Followings {
		p.Follows = append(p.Follows, &models.User{
			Name: following,
		})
	}
	i.insertUser(p)
}

func (i *Inserter) insertUser(user *models.User) {

	const (
		createUserOrAddDetails = `
			MERGE (u:User {name: {name}})
			SET u.realName={realName},u.Bio={bio}, u.AvatarURL={avatarUrl}, u.crawledTS={crawled}
		`
		addRelationsshipAndCreateUserIfNotExisting = `
			MATCH (u1:User {name: {name1}})
			MERGE (u2:User{name: {name2}})
			MERGE (u1)-[:follows]->(u2)
		`
	)

	_, err := i.conn.ExecNeo(createUserOrAddDetails, map[string]interface{}{"name": user.Name, "realName": user.RealName, "avatarUrl": user.AvatarURL, "bio": user.Bio, "crawled": user.CrawledAt})
	if err != nil {
		panic(err)
	}

	// setting relationship to followings
	for _, followed := range user.Follows {
		result, err := i.conn.ExecNeo(addRelationsshipAndCreateUserIfNotExisting, map[string]interface{}{"name1": user.Name, "name2": followed.Name})
		if err != nil {
			panic(err)
		}
		i.handleCreatedUser(result, followed.Name)
	}

}

// Checks if followed user is already in the Queue
func (i *Inserter) handleCreatedUser(result bolt.Result, username string) {
	// Username gets added to Queue only if it just got created
	// rowCount == 2 -> User (only username) and relationship created
	// rowCount == 1 -> relationship created
	if rowsCount, _ := result.RowsAffected(); rowsCount == 2 {
		i.qWriter.WriteMessages(context.Background(), kafka.Message{
			Value: []byte(username),
		})
	}
}

// Close the inserter
func (i *Inserter) Close() {
	i.Stop()
	i.WaitUntilStopped(time.Second * 3)

	i.conn.Close()
	i.qReader.Close()
	i.qWriter.Close()

	i.MarkAsClosed()
}