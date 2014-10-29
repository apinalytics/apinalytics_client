/*
Package goji contains [Goji](https://github.com/zenazn/goji) middleware for reporting events to Apinalytics.

To get started go to http://apinalytics.tanktop.tv/u to get an application Id and write key.
*/
package goji

import (
	"net/http"
	"time"

	cli "github.com/apinalytics/apinalytics_client"
	"github.com/zenazn/goji/web"
)

/*
BuildMiddleware builds middleware for Goji that reports HTTP requests to Apinalytics.

Add it to your Goji mux m as follows.

    m.Use(BuildMiddleWare(myAppId, myWriteKey, "http://apinalytics.tanktop.tv/1/event/", nil))

To add your own data to the events reported add a callback. The main use for this at the moment is to
record the ID of the API consumer.

    callback := func(c *web.C, event *apinalytics_client.AnalyticsEvent, r *http.Request) {
        event.ConsumerId = c.Env["api_user"].(string)
    }

    m.Use(BuildMiddleWare(myAppId, myWriteKey, "http://apinalytics.tanktop.tv/1/event/", callback))

The middleware sets the following event fields: Timestamp, Method, Url, ResponseUS, StatusCode.  It will
also set Function if you record the name of the endpoint/method handling function in c.Env["function"] - e.g.
if you have a function GetEvent that handles GET /api/1/event/:itemtype/ you might record the function name as
follows.

 func GetEvent(c web.C, w http.ResponseWriter, r *http.Request) {
    c.Env["function"] = "GetEvent"
    item_type_str := c.URLParams["itemtype"]
    :
    :
 }

*/
func BuildMiddleWare(applicationId, writeKey, url string,
	callback func(c *web.C, event *cli.AnalyticsEvent, r *http.Request),
) func(c *web.C, h http.Handler) http.Handler {
	sender := cli.NewSender(applicationId, writeKey, url)

	// Return the middleware that references the analytics queue we just made
	return func(c *web.C, h http.Handler) http.Handler {
		handler := func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &cli.StatusTrackingResponseWriter{w, http.StatusOK}

			h.ServeHTTP(ww, r)

			function := c.Env["function"].(string)
			if function == "" {
				function = "unknown"
			}
			event := &cli.AnalyticsEvent{
				Timestamp:  time.Now().Unix(),
				Method:     r.Method,
				Url:        r.RequestURI,
				Function:   function,
				ResponseUS: int(time.Since(start).Nanoseconds() / 1000),
				StatusCode: ww.Status,
			}
			// "path":        r.URL.Path,
			// "user_agent":  r.UserAgent(),
			// "header":      r.Header,
			// Get more data for the analytics event
			if callback != nil {
				callback(c, event, r)
			}

			sender.Queue(event)
		}
		return http.HandlerFunc(handler)
	}
}
