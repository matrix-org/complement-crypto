### mitmproxy

This directory contains code that will be used as a [mitmproxy addon](https://docs.mitmproxy.org/stable/addons-overview/).

How this works:
 - A vanilla `mitmproxy` is run in the same network as the homeservers.
 - It is told to proxy both hs1 and hs2 i.e `mitmdump --mode reverse:http://hs1:8008@3000`
 - It is also told to run a normal proxy, to which a Flask HTTP server is attached.
 - The Flask HTTP server can be used to control mitmproxy at test runtime. This is done via the Controller HTTP API.


### Controller HTTP API

**This is highly experimental and will change without warning.**

`mitmproxy` is run once for all tests. To avoid test pollution, the controller is "locked" for the duration
of a test and must be "unlocked" afterwards. When acquiring the lock, options can be set on `mitmproxy`.

```
POST /options/lock
 {
   "options": {
     "body_size_limit": "3m",
   }
 }
 HTTP/1.1 200 OK
 {
   "reset_id": "some_opaque_string"
 }
```

```
POST /options/unlock
{
   "reset_id": "some_opaque_string"
}
```