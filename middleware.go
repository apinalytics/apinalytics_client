/*
Middleware for Goji that reports HTTP requests to Apinalytics

Add it to your Goji mux m as follows.

    m.Use(BuildMiddleWare(myAppId, nil))

To add your own data to the events reported add a callback.

    callback := func(c *web.C, data map[string]interface{}, r *http.Request) {
        data["my_parameter"] = c.Env["important_info"].(string)
    }

    m.Use(BuildMiddleWare(myAppId, callback))

*/
package apinalytics_client

import (
	"net/http"
	"time"

	"github.com/zenazn/goji/web"
)

/*
Build a middleware function that reports HTTP requests to Apinalytics.

The event the middleware sends is as follows.

    {
        "url":         r.RequestURI,
        "path":        r.URL.Path,
        "method":      r.Method,
        "status_code": w.Status,  // Http status code from response
        "duration_ns": time.Since(start).Nanoseconds(),
        "user_agent":  r.UserAgent(),
        "header":      r.Header,
    }

The callback function allows you to add your own data to the event recorded.  Use it as follows to add events to the "apievents" collection.

    callback := func(c *web.C, data map[string]interface{}, r *http.Request) {
        data["my_parameter"] = c.Env["important_info"].(string)
    }

    m.Use(BuildMiddleWare(myApplicationId, callback))

*/
func BuildMiddleWare(applicationId, url string,
	callback func(c *web.C, event *AnalyticsEvent, r *http.Request),
) func(c *web.C, h http.Handler) http.Handler {
	sender := NewSender(applicationId)

	// Return the middleware that references the analytics queue we just made
	return func(c *web.C, h http.Handler) http.Handler {
		handler := func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &StatusTrackingResponseWriter{w, http.StatusOK}

			h.ServeHTTP(ww, r)

			function := c.Env["function"].(string)
			if function == "" {
				function = "unknown"
			}
			event := &AnalyticsEvent{
				Timestamp:  time.Now(),
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
