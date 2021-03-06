package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/globalsign/mgo"
	"github.com/j-forster/Wazihub-API/mqtt"
	"github.com/j-forster/Wazihub-API/tools"
)

/*
var db *mongo.Client
var collection *mongo.Collection
*/

var db *mgo.Session
var collection *mgo.Collection

var upstream *mqtt.Client

func main() {

	// Remove date and time from logs
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

	tlsCert := flag.String("crt", "", "TLS Cert File (.crt)")
	tlsKey := flag.String("key", "", "TLS Key File (.key)")

	dbAddr := flag.String("db", "localhost:27017", "MongoDB address.")

	upstreamAddr := flag.String("upstream", "", "Upstream server address.")

	flag.Parse()

	////////////////////

	log.Println("WaziHub API Server")
	log.Println("--------------------")

	////////////////////

	log.Printf("[DB   ] Dialing MongoDB at %q...\n", *dbAddr)

	var err error
	db, err = mgo.Dial("mongodb://" + *dbAddr + "/?connect=direct")
	if err != nil {
		db = nil
		log.Println("[DB   ] MongoDB client error:\n", err)
	} else {

		collection = db.DB("Wazihub").C("values")
	}

	////////////////////

	if *upstreamAddr != "" {
		log.Printf("[UP   ] Dialing Upstream at %q...\n", *upstreamAddr)
		upstream, err = mqtt.Dial(*upstreamAddr, "Mario", true, nil, nil)
		if err != nil {
			log.Fatalln(err)
		}
	}

	////////////////////

	if *tlsCert != "" && *tlsKey != "" {

		cert, err := ioutil.ReadFile(*tlsCert)
		if err != nil {
			log.Println("Error reading", *tlsCert)
			log.Fatalln(err)
		}

		key, err := ioutil.ReadFile(*tlsKey)
		if err != nil {
			log.Println("Error reading", *tlsKey)
			log.Fatalln(err)
		}

		pair, err := tls.X509KeyPair(cert, key)
		if err != nil {
			log.Println("TLS/SSL 'X509KeyPair' Error")
			log.Fatalln(err)
		}

		cfg := &tls.Config{Certificates: []tls.Certificate{pair}}

		go ListenAndServeHTTPS(cfg)
		go ListenAndServeMQTTTLS(cfg)
	}

	go ListenAndServerMQTT()
	ListenAndServeHTTP()
}

///////////////////////////////////////////////////////////////////////////////

type ResponseWriter struct {
	http.ResponseWriter
	status int
}

func (resp *ResponseWriter) WriteHeader(statusCode int) {
	resp.status = statusCode
	resp.ResponseWriter.WriteHeader(statusCode)
}

////////////////////

func Serve(resp http.ResponseWriter, req *http.Request) {
	wrapper := ResponseWriter{resp, 200}

	if req.Method == http.MethodPut || req.Method == http.MethodPost {

		body, err := ioutil.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			http.Error(resp, "400 Bad Request", http.StatusBadRequest)
			return
		}
		req.Body = &tools.ClosingBuffer{bytes.NewBuffer(body)}
	}

	router.ServeHTTP(&wrapper, req)

	log.Printf("[%s] (%s) %d %s \"%s\"\n",
		req.Header.Get("X-Tag"),
		req.RemoteAddr,
		wrapper.status,
		req.Method,
		req.RequestURI)

	if cbuf, ok := req.Body.(*tools.ClosingBuffer); ok {
		log.Printf("[DEBUG] Body: %s\n", cbuf.Bytes())
		msg := &mqtt.Message{
			QoS:   0,
			Topic: req.RequestURI[1:],
			Data:  cbuf.Bytes(),
		}

		// if wrapper.status >= 200 && wrapper.status < 300 {
		if req.Method == http.MethodPut || req.Method == http.MethodPost {
			mqttServer.Publish(nil, msg)
		}
		// }
	}

}
