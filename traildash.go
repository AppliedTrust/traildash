package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sqs"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const version = "0.0.10"

const usage = `traildash: easy AWS CloudTrail dashboard

Usage:
	traildash
	traildash --version

Note: traildash uses Environment Vars rather than flags for Docker compatibility.

Required Environment Variables:
	AWS_SQS_URL		AWS SQS URL.

AWS credentials are sourced by (in order): Environment Variables, ~/.aws/credentials, IAM profiles.
	AWS_ACCESS_KEY_ID	AWS Key ID.
	AWS_SECRET_ACCESS_KEY	AWS Secret Key.

Optional Environment Variables:
	AWS_REGION		AWS Region (SQS and S3 regions must match. default: us-east-1).
	ES_URL			ElasticSearch URL (default: http://localhost:9200).
	WEB_LISTEN		Listen IP and port for HTTP/HTTPS interface (default: 0.0.0.0:7000).
	SSL_MODE		"off": disable HTTPS and use HTTP (default)
				"custom": use custom key/cert stored stored in ".tdssl/key.pem" and ".tdssl/cert.pem"
				"selfSigned": use key/cert in ".tdssl", generate an self-signed cert if empty
	SQS_PERSIST		Set to prevent deleting of finished SQS messages - for debugging.
	DEBUG			Enable debugging output.
`

const esPath = "cloudtrail/event"

type sslModeOption int

const (
	SSLoff        sslModeOption = 0
	SSLcustom     sslModeOption = 1
	SSLselfSigned sslModeOption = 2
	SSLcertDir                  = ".tdssl/"
	SSLcertFile                 = SSLcertDir + "cert.pem"
	SSLkeyFile                  = SSLcertDir + "key.pem"
)

var sslModeOptionMap = map[string]sslModeOption{
	"off":        SSLoff,
	"custom":     SSLcustom,
	"selfSigned": SSLselfSigned,
}

type config struct {
	awsKeyId   string
	awsSecret  string
	awsConfig  aws.Config
	region     string
	queueURL   string
	esURL      string
	listen     string
	authUser   string
	authPw     string
	sslMode    sslModeOption
	debugOn    bool
	sqsPersist bool
}

type sqsNotification struct {
	Type             string
	MessageID        string
	TopicArn         string
	Message          string
	Timestamp        string
	SignatureVersion string
	Signature        string
	SigningCertURL   string
	UnsubscribeURL   string
}

type cloudtrailNotification struct {
	S3Bucket      string
	S3ObjectKey   []string
	MessageID     string
	ReceiptHandle string
}

type cloudtrailLog struct {
	Records []cloudtrailRecord
}

type cloudtrailRecord struct {
	EventName          string
	UserAgent          string
	EventID            string
	EventSource        string
	SourceIPAddress    string
	EventType          string
	EventVersion       string
	EventTime          string
	AwsRegion          string
	RequestID          string
	RecipientAccountId string
	UserIdentity       map[string]interface{}
	RequestParameters  map[string]interface{}
	//ResponseElements   string
}

func main() {
	c, err := parseArgs()
	if err != nil {
		fmt.Printf("Error parsing arguments: %s\n\n", err.Error())
		fmt.Println(usage)
		os.Exit(1)
	}

	go c.workLogs()
	go c.serveKibana()

	log.Print("Started")
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case s := <-sig:
			log.Fatalf("Signal (%d) received, stopping", s)
		}
	}
	log.Print("Exiting!")
}

// serveKibana runs a webserver for 1. kibana and 2. elasticsearch proxy
func (c *config) serveKibana() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		webStaticHandler(w, r)
	})
	http.HandleFunc("/es/", c.proxyHandler)
	if c.sslMode == SSLoff {
		http.ListenAndServe(c.listen, nil)
	} else {
		http.ListenAndServeTLS(c.listen, SSLcertFile, SSLkeyFile, nil)
	}
	log.Print("Web server exit")
}

// webStaticHandler serves embedded static web files (js&css)
func webStaticHandler(w http.ResponseWriter, r *http.Request) {
	assetPath := "kibana/" + r.URL.Path[1:]
	if assetPath == "kibana/" {
		assetPath = "kibana/index.html"
	}
	staticAsset, err := Asset(assetPath)
	if err != nil {
		log.Printf("Kibana web error: %s", err.Error())
		http.NotFound(w, r)
		return
	}
	headers := w.Header()
	if strings.HasSuffix(assetPath, ".js") {
		headers["Content-Type"] = []string{"application/javascript"}
	} else if strings.HasSuffix(assetPath, ".css") {
		headers["Content-Type"] = []string{"text/css"}
	} else if strings.HasSuffix(assetPath, ".html") {
		headers["Content-Type"] = []string{"text/html"}
	}
	io.Copy(w, bytes.NewReader(staticAsset))
}

// proxyHandler securely proxies requests to the ElasticSearch instance
func (c *config) proxyHandler(w http.ResponseWriter, r *http.Request) {
	u, err := url.Parse(c.esURL)
	if err != nil {
		log.Printf("URL err: %s", err.Error())
		http.Error(w, err.Error(), 500)
		return
	}

	if firewallES(r) {
		c.debug("Permitting ES %s request: %s", r.Method, r.RequestURI)
	} else {
		c.debug("Refusing ES %s request: %s", r.Method, r.RequestURI)
		http.Error(w, "Permission denied", 403)
		return
	}

	client := &http.Client{}
	req := r
	req.RequestURI = ""
	req.Host = u.Host
	req.URL.Host = req.Host
	req.URL.Scheme = u.Scheme
	req.URL.Path = strings.TrimPrefix(req.URL.Path, "/es")

	resp, err := client.Do(req)
	if err != nil {
		c.debug("Proxy err: %s", err.Error())
		http.Error(w, err.Error(), 500)
		return
	}
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err := resp.Body.Close(); err != nil {
		c.debug("Can't close response body %v", err)
	}
	//c.debug("Copied %v bytes to client error=%v", nr, err)
}

// firewallES provides a basic "firewall" for ElasticSearch
func firewallES(r *http.Request) bool {
	switch r.Method {
	case "GET":
		return true
	case "POST":
		parts := strings.SplitN(r.RequestURI, "?", 2)
		if strings.HasSuffix(parts[0], "_search") {
			return true
		}
	case "PUT":
		if strings.HasPrefix(r.RequestURI, "/es/kibana-int/dashboard/") {
			return true
		}
	default:
		return false
	}
	return false
}

// workLogs fetches and loads logs from SQS
func (c *config) workLogs() {
	for {
		// fetch a message from SQS
		m, err := c.dequeue()
		if err != nil {
			kerblowie("Error dequeing from SQS: %s", err.Error())
			continue
		} else if m == nil {
			log.Printf("Empty queue... polling for 20 seconds.")
			continue
		}
		if len(m.S3ObjectKey) < 1 {
			kerblowie("Error dequeing from SQS: S3ObjectKey empty.  Please grab the contents of SQS message ID sqs://%s and report in a GitHub issue.  Thanks!!", m.MessageID)
			continue
		}

		// download from S3
		records, err := c.download(m)
		if err != nil {
			kerblowie("Error downloading from S3: %s", err.Error())
			continue
		}
		c.debug("Downloaded %d records from sqs://%s [s3://%s/%s]", len(*records), m.MessageID, m.S3Bucket, m.S3ObjectKey[0])

		// load into elasticsearch
		if err = c.load(records); err != nil {
			kerblowie("Error uploading to ElasticSearch: %s", err.Error())
			continue
		}
		c.debug("Uploaded sqs://%s [s3://%s/%s] to es://%s", m.MessageID, m.S3Bucket, m.S3ObjectKey[0], esPath)

		// delete message from sqs
		if c.sqsPersist {
			c.debug("NOT DELETING sqs://%s [s3://%s/%s]", m.MessageID, m.S3Bucket, m.S3ObjectKey[0])
		} else {
			if err = c.deleteSQS(m); err != nil {
				kerblowie("Error deleting from SQS queue: %s", err.Error())
				continue
			}
			c.debug("Deleted sqs://%s [s3://%s/%s]", m.MessageID, m.S3Bucket, m.S3ObjectKey[0])
		}
		log.Printf("Loaded CloudTrail file with %d records.", len(*records))
	}
}

// dequeue fetches an item from SQS
func (c *config) dequeue() (*cloudtrailNotification, error) {
	numRequested := 1
	sess := session.New(awsConfig)
	q := sqs.New(sess)

	req := sqs.ReceiveMessageInput{
		QueueURL:            aws.String(c.queueURL),
		MaxNumberOfMessages: aws.Int64(int64(numRequested)),
		WaitTimeSeconds:     aws.Int64(20), // max allowed
	}
	resp, err := q.ReceiveMessage(&req)
	if err != nil {
		return nil, fmt.Errorf("SQS ReceiveMessage error: %s", err.Error())
	}
	//c.debug("Received %d messsage from SQS.", len(resp.Messages))
	if len(resp.Messages) > numRequested {
		return nil, fmt.Errorf("Expected %d but got %d messages.", numRequested, len(resp.Messages))
	} else if len(resp.Messages) == 0 {
		return nil, nil
	}
	m := resp.Messages[0]
	body := *m.Body

	not := sqsNotification{}
	if err := json.Unmarshal([]byte(body), &not); err != nil {
		return nil, fmt.Errorf("SQS message JSON error [id: %s]: %s", not.MessageID, err.Error())
	}

	n := cloudtrailNotification{}
	n.MessageID = not.MessageID
	n.ReceiptHandle = *m.ReceiptHandle
	if not.Message == "CloudTrail validation message." { // swallow validation messages
		if err = c.deleteSQS(&n); err != nil {
			return nil, fmt.Errorf("Error deleting CloudTrail validation message [id: %s]: %s", not.MessageID, err.Error())
		}
		return nil, fmt.Errorf("Deleted CloudTrail validation message id %s", not.MessageID)
	} else if err := json.Unmarshal([]byte(not.Message), &n); err != nil {
		return nil, fmt.Errorf("CloudTrail JSON error [id: %s]: %s", not.MessageID, err.Error())
	}
	return &n, nil
}

// download fetches the CloudTrail logfile from S3 and parses it
func (c *config) download(m *cloudtrailNotification) (*[]cloudtrailRecord, error) {
	if len(m.S3ObjectKey) != 1 {
		return nil, fmt.Errorf("Expected one S3 key but got %d", len(m.S3ObjectKey[0]))
	}
	sess := session.New(awsConfig)
	s := s3.New(sess)
	q := s3.GetObjectInput{
		Bucket: aws.String(m.S3Bucket),
		Key:    aws.String(m.S3ObjectKey[0]),
	}
	o, err := s.GetObject(&q)
	if err != nil {
		return nil, err
	}
	b, err := ioutil.ReadAll(o.Body)
	if err != nil {
		return nil, err
	}

	logfile := cloudtrailLog{}

	if err := json.Unmarshal(b, &logfile); err != nil {
		return nil, fmt.Errorf("Error unmarshaling cloutrail JSON: %s", err.Error())
	}

	return &logfile.Records, nil
}

// load stores a group of cloudtrail records into ElasticSearch
func (c *config) load(records *[]cloudtrailRecord) error {
	bulk := ""
	for _, r := range *records { // build file for bulk upload to ES
		j, err := json.Marshal(r)
		if err != nil {
			return err
		}
		bulk += fmt.Sprintf(`{ "index": { "_id" : "%s" }}`+"\n", r.EventID)
		bulk += string(j) + "\n"
	}
	url := fmt.Sprintf("%s/%s/_bulk", c.esURL, esPath)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(bulk)))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.Status != "200 OK" {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("Error response from Elasticsearch: %s %s", resp.Status, string(body))
	}
	return nil
}

// deleteSQS removes a completed notification from the queue
func (c *config) deleteSQS(m *cloudtrailNotification) error {
	sess := session.New(awsConfig)
	q := sqs.New(sess)
	req := sqs.DeleteMessageInput{
		QueueURL:      aws.String(c.queueURL),
		ReceiptHandle: aws.String(m.ReceiptHandle),
	}
	_, err := q.DeleteMessage(&req)
	if err != nil {
		return err
	}
	return nil
}

// parseArgs handles CLI flags and env vars
func parseArgs() (*config, error) {
	c := config{}

	var verPtr bool
	var helpPtr bool
	flag.BoolVar(&verPtr, "version", false, "print version")
	flag.BoolVar(&verPtr, "v", false, "print version")
	flag.BoolVar(&helpPtr, "help", false, "print usage")
	flag.BoolVar(&helpPtr, "h", false, "print usage")

	flag.Parse()
	if verPtr {
		fmt.Println(version)
		os.Exit(0)
	} else if helpPtr {
		fmt.Println(usage)
		os.Exit(0)
	}

	c.queueURL = os.Getenv("AWS_SQS_URL")
	if len(c.queueURL) < 1 {
		return nil, fmt.Errorf("Must specify SQS url with -Q flag or by setting AWS_SQS_URL env var.")
	}
	c.awsKeyId = os.Getenv("AWS_ACCESS_KEY_ID")
	c.awsSecret = os.Getenv("AWS_SECRET_ACCESS_KEY")
	c.region = os.Getenv("AWS_REGION")
	if len(c.region) < 1 {
		c.region = "us-east-1"
	}
	c.awsConfig = aws.Config{Region: aws.String(c.region)}
	c.esURL = os.Getenv("ES_URL")
	if len(c.esURL) < 1 {
		c.esURL = "http://127.0.0.1:9200"
	}

	c.listen = os.Getenv("WEB_LISTEN")
	if len(c.listen) < 1 {
		c.listen = "0.0.0.0:7000"
	}
	if len(os.Getenv("DEBUG")) > 0 {
		c.debugOn = true
	}
	if len(os.Getenv("SQS_PERSIST")) > 0 {
		c.sqsPersist = true
	}

	c.sslMode = SSLoff
	if len(os.Getenv("SSL_MODE")) > 0 {
		var ok bool
		c.sslMode, ok = sslModeOptionMap[os.Getenv("SSL_MODE")]
		if !ok {
			return nil, fmt.Errorf("Invalid SSL_MODE.  Must be one of 'off', 'selfSigned', or 'custom'.")
		}
	}

	if c.sslMode != SSLoff {
		// look for existing ".tdssl/key.pem" and ".tdssl/cert.pem"
		_, keyErr := os.Stat(SSLkeyFile)
		_, certErr := os.Stat(SSLcertFile)
		if os.IsNotExist(keyErr) && os.IsNotExist(certErr) && c.sslMode == SSLselfSigned {
			if _, dirErr := os.Stat(SSLcertDir); os.IsNotExist(dirErr) {
				if err := os.Mkdir(SSLcertDir, 0700); err != nil {
					return nil, fmt.Errorf("Error creating SSL cert directory at %s: %s", SSLcertDir, err.Error())
				}
			}
			if err := generateCert(SSLcertFile, SSLkeyFile); err != nil {
				return nil, fmt.Errorf("Error generating a self-signed SSL cert: %s", err.Error())
			}
			log.Printf("Created new self-signed SSL cert in %s.", SSLcertDir)
		} else if os.IsNotExist(keyErr) || os.IsNotExist(certErr) {
			return nil, fmt.Errorf("SSL key or cert missing. Expected at %s and %s", SSLcertFile, SSLkeyFile)
		}
	}

	return &c, nil
}

// debug reports stuff if debugging is on
func (c *config) debug(format string, m ...interface{}) {
	if c.debugOn {
		log.Printf(format, m...)
	}
}

// kerblowie handles API failures "gracefully"... hah
func kerblowie(format string, s ...interface{}) {
	log.Printf(format, s...)
	time.Sleep(5 * time.Second)
}

// copyHeaders copies HTTP headers to proxy responses
func copyHeaders(dst, src http.Header) {
	for k, _ := range dst {
		dst.Del(k)
	}
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
