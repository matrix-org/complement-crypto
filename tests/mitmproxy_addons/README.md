### mitmproxy

This directory contains code that will be used as a [mitmproxy addon](https://docs.mitmproxy.org/stable/addons-overview/).

How this works:
 - A vanilla `mitmproxy` is run in the same network as the homeservers.
 - It is told to proxy both hs1 and hs2 i.e `mitmdump --mode reverse:http://hs1:8008@3000`
 - It is also told to run a normal proxy, to which a Flask HTTP server is attached.
 - The Flask HTTP server can be used to control mitmproxy at test runtime. This is done via the Controller HTTP API.


### Controller HTTP API

`mitmproxy` is run once for all tests. To avoid test pollution, the controller is "locked" for the duration
of a test and must be "unlocked" afterwards. When acquiring the lock, options can be set on `mitmproxy`.

```
POST /options/lock
 {
   "options": {
     "body_size_limit": "3m",
     "callback": {
       "callback_response_url": "http://host.docker.internal:445566"
     }
   }
 }
 HTTP/1.1 200 OK
 {
   "reset_id": "some_opaque_string"
 }
```
Any [option](https://docs.mitmproxy.org/stable/concepts-options/) can be specified in the
`options` object, not just Complement specific addons.

```
POST /options/unlock
{
   "reset_id": "some_opaque_string"
}
```

### Callback addon

A [mitmproxy addon](https://docs.mitmproxy.org/stable/addons-examples/) bolts on custom
functionality to mitmproxy. This typically involves using the
[Event Hooks API](https://docs.mitmproxy.org/stable/api/events.html) to listen for
[HTTP flows](https://docs.mitmproxy.org/stable/api/mitmproxy/http.html#HTTPFlow).

The `callback` addon is a Complement-Crypto specific addon which calls a client provided URL
mid-flow, with a JSON object containing information about the HTTP flow. The caller can then
return another JSON object which can modify the response in some way.

Available configuration options:
 - `callback_request_url`
 - `callback_response_url`
 - `filter`

mitmproxy will call the callback URL with the following JSON object:
```js
{
   method: "GET|PUT|...",
   access_token: "syt_11...",
   url: "http://hs1/_matrix/client/...",
   request_body: { some json object or null if no body },

   // these fields will be missing for `callback_request_url` callbacks as the request
   // has not yet been sent to the server.
   response_body: { some json object },
   response_code: 200,
}
```
The callback server can then return optional keys to replace parts of the response:
```js
{
   respond_status_code: 200,
   respond_body: { "some": "json_object" }
}
```
