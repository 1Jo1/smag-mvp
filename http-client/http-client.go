package http_client

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/codeuniversity/smag-mvp/models"
	"github.com/segmentio/kafka-go"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var userAccountInfoUrl = "https://instagram.com/%s/?__a=1"
var userAccountMediaUrl = "https://www.instagram.com/graphql/query/?query_hash=58b6785bea111c67129decbe6a448951&variables=%s"
var userPostsCommentUrl = "https://www.instagram.com/graphql/query/?query_hash=865589822932d1b43dfe312121dd353a&variables=%s"

type HttpClient struct {
	browserAgent             BrowserAgent
	localAddressesReachLimit map[string]bool
	currentAddress           string
	client                   *http.Client
	renewedAddressQReader    *kafka.Reader
	reachedLimitQWriter      *kafka.Writer
	instanceId               string
}

func NewHttpClient(localAddressCount int, kafkaAddress string) *HttpClient {
	client := &HttpClient{}
	client.renewedAddressQReader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaAddress},
		GroupID:        "instagram_group1",
		Topic:          "renewed_elastic_ip",
		CommitInterval: time.Minute * 10,
	})

	client.reachedLimitQWriter = kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{kafkaAddress},
		Topic:    "reached_limit",
		Balancer: &kafka.LeastBytes{},
		Async:    true,
	})

	data, err := ioutil.ReadFile("useragents.json")
	if err != nil {
		panic(err)
	}
	var userAgent BrowserAgent
	errJson := json.Unmarshal(data, &userAgent)

	if errJson != nil {
		panic(errJson)
	}
	client.browserAgent = userAgent
	client.localAddressesReachLimit = make(map[string]bool)
	addresses := getLocalIpAddresses(localAddressCount)
	//addresses := []string{"192.168.178.41"}

	for _, localIp := range addresses {
		client.localAddressesReachLimit[localIp] = true
	}

	client.client, err = client.getBoundAddressClient(addresses[0])

	if err != nil {
		panic(err)
	}

	client.instanceId = getAmazonInstanceId()
	fmt.Println("localAddressesReachLimit: ", client.localAddressesReachLimit)
	return client
}

func getAmazonInstanceId() string {
	resp, err := http.Get("http://169.254.169.254/latest/meta-data/instance-id")
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		fmt.Println("Error: ", err)
		panic(err)
	}
	return string(body)
}

func isNetworkInterfaces(name string) bool {
	matched, err := regexp.MatchString("eth[0-9]*$", name)

	if err != nil {
		panic(err)
	}

	return matched
}

func isIpv4Address(ip string) bool {
	matched, err := regexp.MatchString("^(?:[0-9]{1,3}\\.){3}[0-9]{1,3}$", ip)

	if err != nil {
		panic(err)
	}

	return matched
}

func getLocalIpAddresses(count int) []string {
	interfaces, err := net.Interfaces()

	if err != nil {
		fmt.Println("Get Network Interfaces Error: ")
		panic(err)
	}

	var localAddresses []string
	for _, networkInterface := range interfaces {
		if isNetworkInterfaces(networkInterface.Name) {
			addrs, err := networkInterface.Addrs()
			if err != nil {
				fmt.Println("Error Addrs: ", err)
				panic(err)
			}
			for _, address := range addrs {
				ip := strings.Split(address.String(), "/")
				if isIpv4Address(ip[0]) {
					localAddresses = append(localAddresses, ip[0])
				}
			}
		}
	}

	if len(localAddresses) < count {
		panic(fmt.Sprintf("Not Enough Local Ip Addresses, Requirement: %d \n", count))
	}

	fmt.Println("All LocalAddresses: ", localAddresses)
	return localAddresses[:count]
}

func (h *HttpClient) getBoundAddressClient(localIp string) (*http.Client, error) {
	localAddr, err := net.ResolveIPAddr("ip", localIp)

	if err != nil {
		return nil, err
	}

	localTCPAddr := net.TCPAddr{
		IP: localAddr.IP,
	}

	d := net.Dialer{
		LocalAddr: &localTCPAddr,
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         d.DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	return &http.Client{Transport: tr}, nil
}

func (h *HttpClient) getClient() *http.Client {
	return h.client
}

func (h *HttpClient) ScrapeAccountInfo(username string) (models.InstagramAccountInfo, error) {
	var userAccountInfo models.InstagramAccountInfo
	url := fmt.Sprintf(userAccountInfoUrl, username)
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return userAccountInfo, err
	}
	h.GetHeaders(request)

	response, err := h.client.Do(request)
	if err != nil {
		return userAccountInfo, err
	}
	if response.StatusCode != 200 {
		return userAccountInfo, &HttpStatusError{fmt.Sprintf("Error HttpStatus: %s", response.StatusCode)}
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return userAccountInfo, err
	}
	err = json.Unmarshal(body, &userAccountInfo)
	if err != nil {
		return userAccountInfo, err
	}
	return userAccountInfo, nil
}

func (h *HttpClient) ScrapeProfileMedia(userId string, endCursor string) (models.InstagramMedia, error) {
	var instagramMedia models.InstagramMedia

	type Variables struct {
		Id    string `json:"id"`
		First int    `json:"first"`
		After string `json:"after"`
	}
	variable := &Variables{userId, 12, endCursor}
	variableJson, err := json.Marshal(variable)
	fmt.Println(string(variableJson))
	if err != nil {
		return instagramMedia, err
	}
	queryEncoded := url.QueryEscape(string(variableJson))
	url := fmt.Sprintf(userAccountMediaUrl, queryEncoded)

	request, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return instagramMedia, err
	}
	response, err := h.client.Do(request)
	if err != nil {
		return instagramMedia, err
	}
	if response.StatusCode != 200 {
		return instagramMedia, &HttpStatusError{fmt.Sprintf("Error HttpStatus: %s", response.StatusCode)}
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return instagramMedia, err
	}
	err = json.Unmarshal(body, &instagramMedia)
	if err != nil {
		return instagramMedia, err
	}
	return instagramMedia, nil
}

func (h *HttpClient) ScrapePostComments(shortCode string) (models.InstaPostComments, error) {
	var instaPostComment models.InstaPostComments
	type Variables struct {
		Shortcode           string `json:"shortcode"`
		ChildCommentCount   int    `json:"child_comment_count"`
		FetchCommentCount   int    `json:"fetch_comment_count"`
		ParentCommentCount  int    `json:"parent_comment_count"`
		HasThreadedComments bool   `json:"has_threaded_comments"`
	}

	variable := &Variables{shortCode, 3, 40, 24, true}
	variableJson, err := json.Marshal(variable)
	if err != nil {
		return instaPostComment, err
	}

	queryEncoded := url.QueryEscape(string(variableJson))
	url := fmt.Sprintf(userPostsCommentUrl, queryEncoded)

	request, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return instaPostComment, err
	}
	response, err := h.client.Do(request)
	if err != nil {
		return instaPostComment, err
	}
	if response.StatusCode != 200 {
		return instaPostComment, &HttpStatusError{fmt.Sprintf("Error HttpStatus: %s", response.StatusCode)}
	}
	fmt.Println("ScrapePostComments got response")
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return instaPostComment, err
	}
	err = json.Unmarshal(body, &instaPostComment)
	if err != nil {
		return instaPostComment, err
	}
	return instaPostComment, nil
}

func (h *HttpClient) WithRetries(times int, f func() error) error {
	var err error
	for i := 0; i < times; i++ {
		err = f()
		if err == nil {
			return nil
		}

		fmt.Println(err)
		foundAddress, err := h.checkIfIPReachedTheLimit(err)
		fmt.Println("FoundAddress: ", foundAddress)
		if err != nil {
			fmt.Println(err)
		}
		if foundAddress {
			times++
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

func (h *HttpClient) checkIfIPReachedTheLimit(err error) (bool, error) {
	fmt.Println("checkIfIPReachedTheLimit")
	switch t := err.(type) {
	case *json.SyntaxError:
		fmt.Println("SyntaxError")
		addresses, foundAddress := h.checkAvailableAddresses()

		if foundAddress {
			return true, nil
		}
		if h.localAddressesReachLimit[h.currentAddress] == false {
			err := h.sendRenewElasticIpRequestToAmazonService(addresses)
			if err != nil {
				return false, err
			}

			renewedAddresses := models.RenewingAddresses{}

			fmt.Println("H.InstanceId: ", h.instanceId)
			for renewedAddresses.InstanceId != h.instanceId {

				renewedAddresses, err := h.waitForRenewElasticIpRequest()
				if err != nil {
					return false, err
				}

				if renewedAddresses.InstanceId == h.instanceId {
					for ip := range h.localAddressesReachLimit {
						h.localAddressesReachLimit[ip] = true
					}
					return true, nil
				}
			}
		}
	case *json.UnmarshalTypeError:
		fmt.Println("UnmarshalTypeError")
	case *json.InvalidUnmarshalError:
		fmt.Println("InvalidUnmarchedError")
	case *json.UnsupportedTypeError:
		fmt.Println("UnsupportedTypeError")
	case *HttpStatusError:
		fmt.Println("HttpStatusError")
		addresses, foundAddress := h.checkAvailableAddresses()

		if foundAddress {
			return true, nil
		}
		if h.localAddressesReachLimit[h.currentAddress] == false {
			err := h.sendRenewElasticIpRequestToAmazonService(addresses)
			if err != nil {
				return false, err
			}

			renewedAddresses := models.RenewingAddresses{}

			fmt.Println("H.InstanceId: ", h.instanceId)
			for renewedAddresses.InstanceId != h.instanceId {

				renewedAddresses, err := h.waitForRenewElasticIpRequest()
				if err != nil {
					return false, err
				}

				if renewedAddresses.InstanceId == h.instanceId {
					for ip := range h.localAddressesReachLimit {
						h.localAddressesReachLimit[ip] = true
					}
					return true, nil
				}
			}
		}
	default:
		fmt.Println("Found Wrong Json Type Error ", t)
		return false, err
	}
	fmt.Println("checkIfIPReachedTheLimit is not working!!!")
	return false, err
}

func (h *HttpClient) checkAvailableAddresses() ([]string, bool) {
	h.localAddressesReachLimit[h.currentAddress] = false
	var addresses []string
	var err error
	for ip := range h.localAddressesReachLimit {
		addresses = append(addresses, ip)
		if h.localAddressesReachLimit[ip] == true {
			h.currentAddress = ip
			h.client, err = h.getBoundAddressClient(ip)
			if err != nil {
				panic(err)
			}
			fmt.Println("Update Client")
			return addresses, true
		}
	}
	return addresses, false
}
func (h *HttpClient) sendRenewElasticIpRequestToAmazonService(addresses []string) error {
	renewAddresses := models.RenewingAddresses{
		InstanceId: h.instanceId,
		LocalIps:   addresses,
	}

	renewAdressesJson, err := json.Marshal(renewAddresses)
	if err != nil {
		return err
	}
	h.reachedLimitQWriter.WriteMessages(context.Background(), kafka.Message{Value: renewAdressesJson})
	return nil
}

func (h *HttpClient) waitForRenewElasticIpRequest() (*models.RenewingAddresses, error) {
	fmt.Println("waitForRenewElasticIpRequest")
	message, err := h.renewedAddressQReader.FetchMessage(context.Background())
	fmt.Println("waitForRenewElasticIpRequest Finished: ")
	if err != nil {
		fmt.Println("waitForRenewElasticIpRequest error")
		return nil, err
	}
	fmt.Println("Wait Message Time: ", message.Time)

	var renewedAddresses models.RenewingAddresses
	err = json.Unmarshal(message.Value, &renewedAddresses)
	if err != nil {
		return nil, err
	}

	h.renewedAddressQReader.CommitMessages(context.Background(), message)
	return &renewedAddresses, err
}

type BrowserAgent []struct {
	UserAgents string `json:"useragent"`
}

func (h *HttpClient) getRandomUserAgent() string {
	randomNumber := rand.Intn(len(h.browserAgent))
	return h.browserAgent[randomNumber].UserAgents
}

func (h *HttpClient) Close() {
	h.renewedAddressQReader.Close()
	h.reachedLimitQWriter.Close()
}

func (h *HttpClient) GetHeaders(request *http.Request) {
	request.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3")
	request.Header.Add("Accept-Charset", "utf-8")
	request.Header.Add("Accept-Language", "de-DE,de;q=0.9,en-US;q=0.8,en;q=0.7")
	request.Header.Add("Cache-Control", "no-cache")
	request.Header.Add("Content-Type", "application/json; charset=utf-8")
	request.Header.Add("User-Agent", h.getRandomUserAgent())
}
