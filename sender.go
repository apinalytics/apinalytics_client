/*
Send events to apinalytics.io asynchronously in batches.
*/
package apinalytics_client

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	// Server URL
	// url string = "http://127.0.0.1:7998/1/event/"
	// The size of the queue to the background goroutine
	channel_size int = 100
	// The background routine will send batches of events up to this size
	send_threshold int = 90
)

// Type for queuing events to the background
type AnalyticsEvent struct {
	// Timestamp for this event in seconds since 1 Jan 1970 UTC
	Timestamp int64 `json:"timestamp"`
	// Identifier for the API consumer
	ConsumerId string `json:"consumer_id"`
	// HTTP Method used ("GET", "POST", etc.)
	Method string `json:"method"`
	// Url used (including parameters)
	Url string `json:"url"`
	// Name of the function invoked.
	Function string `json:"function",omitempty`
	// API response time in microseconds
	ResponseUS int `json:"response_us"`
	// HTTP status code
	StatusCode int `json:"status_code"`
	// Arbitrary key, value pairs to report.  Not yet implemented
	Data map[string]string `json:"data",omitempty`
}

type Sender struct {
	applicationId string
	writeKey      string
	url           string               // The url to post events too, including project details
	events        []*AnalyticsEvent    // For batching events as we pull them off the channel
	count         int                  // Number of events batched and ready to send
	channel       chan *AnalyticsEvent // For queuing events to the background
	done          chan bool            // For clean exiting
}

/*
Create a new Sender.

This creates a background goroutine to aggregate and send your events.

 applicationId - Identifies the application generating the events.
 url           - URL of the Apinalytics service
*/
func NewSender(applicationId, writeKey, url string) *Sender {
	sender := &Sender{
		applicationId: applicationId,
		writeKey:      writeKey,
		channel:       make(chan *AnalyticsEvent, channel_size),
		done:          make(chan bool),
	}
	sender.url = url
	sender.reset()
	go sender.run()
	return sender
}

/*
Queue events to be sent to Apinalytics

info can be anything that is JSON serializable.  Events are immediately queued to a background goroutine for sending.  The
background routine will send everything that's queued to it in a batch, then wait for new data.

The upshot is that if you send events slowly they will be sent immediately and individually, but if you send events quickly they will be batched
*/
func (sender *Sender) Queue(event *AnalyticsEvent) {
	sender.channel <- event
}

/*
Close the sender and wait for queued events to be sent
*/
func (sender *Sender) Close() {
	// Closing the channel signals the background thread to exit
	close(sender.channel)
	// Wait for the background thread to signal it has flushed all events and exited
	<-sender.done
}

// Add an event to the map that's used to batch events
func (sender *Sender) add(event *AnalyticsEvent) bool {
	if event == nil {
		// nil event, don't add
		return false
	}
	sender.events = append(sender.events, event)
	sender.count++

	if sender.count > send_threshold {
		sender.send()
	}
	return true
}

// Reset the event map that's used to batch events
func (sender *Sender) reset() {
	sender.events = make([]*AnalyticsEvent, 0, 10)
	sender.count = 0
}

// Send the events currently in sender.events
func (sender *Sender) send() {
	if sender.count == 0 {
		return
	}
	// Whether we can send the events or not, we dump them before exiting this function
	defer sender.reset()

	// Convert data to JSON
	data, err := json.Marshal(sender.events)
	if err != nil {
		log.Printf("Couldn't marshal json for analytics. %v\n", err)
		return
	}

	start := time.Now()
	req, err := http.NewRequest("POST", sender.url, strings.NewReader(string(data)))
	if err != nil {
		log.Printf("Failed to build analytics POST. %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-User", sender.applicationId)
	req.Header.Set("X-Auth-Key", sender.writeKey)
	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Failed to post analytics events.  %v\n", err)
		return
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		log.Printf("Failure return for analytics post.  %d, %s\n", rsp.StatusCode, rsp.Status)
	} else {
		// TODO: remove once analytics has bedded in
		log.Printf("analytics sent in %v\n", time.Since(start))
	}
}

func (sender *Sender) run() {
	var event *AnalyticsEvent

	// Block for the first event, once we have one event we try to drain everthing left
	for event = range sender.channel {
		sender.add(event)

		// Select with a default case is essentially a non-blocking read from the channel
	Loop:
		for {
			select {
			case event = <-sender.channel:
				// Add the event to those we are batching
				if !sender.add(event) {
					break Loop
				}

			default:
				// Nothing to batch at present.  Send our events if we have any, then go back to block until something
				// shows up
				break Loop
			}
		}
		// Send what we have batched
		sender.send()
	}

	// Indicate that this thread is over
	sender.done <- true
	log.Printf("Analytics exited\n")
}
