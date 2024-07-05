from typing import Optional
import asyncio
import aiohttp
import json

from mitmproxy import ctx, flowfilter
from mitmproxy.http import Response
from controller import MITM_DOMAIN_NAME
from urllib.request import urlopen, Request
from urllib.error import HTTPError, URLError
from datetime import datetime

# Callback will intercept a request and/or response and send a POST request to the provided url, with
# the following JSON object. Supports filters: https://docs.mitmproxy.org/stable/concepts-filters/
# {
#   method: "GET|PUT|...",
#   access_token: "syt_11...",
#   url: "http://hs1/_matrix/client/...",
#   request_body: { some json object or null if no body },
#   response_body: { some json object },
#   response_code: 200,
# }
# The response to this request can control what gets returned to the client. The response object:
# {
#    "respond_status_code": 200,
#    "respond_body": { "some": "json_object" }
# }
# If {} is sent back, the response is not modified. Likewise, if `respond_body` is set but
# `respond_status_code` is not, only the response body is modified, not the status code, and vice versa.
#
# To use this addon, configure it with these fields:
#  - callback_request_url: the URL to send outbound requests to. This allows callbacks to intercept
#                          requests BEFORE they reach the server. The request/response struct in this
#                          callback is the same as `callback_response_url`, except `response_body`
#                          and `response_code` will be missing as the request hasn't been processed
#                          yet. To block the request from reaching the server, the callback needs to
#                          provide both `respond_status_code` and `respond_body`.
#  - callback_response_url: the URL to send inbound responses to. This allows callbacks to modify
#                           response content.
#  - filter: the mitmproxy filter to apply. If unset, ALL requests are eligible to go to the callback
#            server.
class Callback:
    def __init__(self):
        self.reset()
        self.matchall = flowfilter.parse(".")
        self.filter: Optional[flowfilter.TFilter] = self.matchall

    def reset(self):
        self.config = {
            "callback_request_url": "",
            "callback_response_url": "",
            "filter": None,
        }

    def load(self, loader):
        loader.add_option(
            name="callback",
            typespec=dict,
            default={
                "callback_request_url": "",
                "callback_response_url": "",
                "filter": None,
            },
            help="Change the callback url, with an optional filter",
        )

    def configure(self, updates):
        if "callback" not in updates:
            self.reset()
            return
        if ctx.options.callback is None:
            self.reset()
            return
        self.config = ctx.options.callback
        new_filter = self.config.get('filter', None)
        print(f"callback req_url={self.config.get('callback_request_url')} res_url={self.config.get('callback_response_url')} filter={new_filter}")
        if new_filter:
            self.filter = flowfilter.parse(new_filter)
        else:
            self.filter = self.matchall

    async def request(self, flow):
        # always ignore the controller
        if flow.request.pretty_host == MITM_DOMAIN_NAME:
            return
        if self.config.get("callback_request_url", "") == "":
            return # ignore requests if we aren't told a url
        if not flowfilter.match(self.filter, flow):
            return # ignore requests which don't match the filter
        try: # e.g GET requests have no req body
            req_body = flow.request.json()
        except:
            req_body = None
        print(f'{datetime.now().strftime("%H:%M:%S.%f")} hitting request callback for {flow.request.url}')
        callback_body = {
            "method": flow.request.method,
            "access_token": flow.request.headers.get("Authorization", "").removeprefix("Bearer "),
            "url": flow.request.url,
            "request_body": req_body,
        }
        await self.send_callback(flow, self.config["callback_request_url"], callback_body)

    async def response(self, flow):
        # always ignore the controller
        if flow.request.pretty_host == MITM_DOMAIN_NAME:
            return
        if self.config.get("callback_response_url","") == "":
            return # ignore responses if we aren't told a url
        if flowfilter.match(self.filter, flow):
            try: # e.g GET requests have no req body
                req_body = flow.request.json()
            except:
                req_body = None
            try: # e.g OPTIONS responses have no res body
                res_body = flow.response.json()
            except:
                res_body = None
            print(f'{datetime.now().strftime("%H:%M:%S.%f")} hitting response callback for {flow.request.url}')
            callback_body = {
                "method": flow.request.method,
                "access_token": flow.request.headers.get("Authorization", "").removeprefix("Bearer "),
                "url": flow.request.url,
                "response_code": flow.response.status_code,
                "request_body": req_body,
                "response_body": res_body,
            }
            await self.send_callback(flow, self.config["callback_response_url"], callback_body)

    async def send_callback(self, flow, url: str, body: dict):
        try:
            # use asyncio so we don't block other unrelated requests from being processed
            async with aiohttp.request(
                method="POST",
                url=url,
                timeout=aiohttp.ClientTimeout(total=10),
                headers={"Content-Type": "application/json"},
                json=body,
            ) as response:
                print(f'{datetime.now().strftime("%H:%M:%S.%f")} callback for {flow.request.url} returned HTTP {response.status}')
                if response.content_type != 'application/json':
                    err_response_body = await response.text()
                    print(f'ERR: callback server returned non-json: {err_response_body}')
                    raise Exception("callback server content-type: " + response.content_type)
                test_response_body = await response.json()
                # if the response includes some keys then we are modifying the response on a per-key basis.
                if len(test_response_body) > 0:
                    # use what fields were provided preferentially.
                    # For requests: both fields must be provided so the default case won't execute.
                    # For responses: fields are optional but the default case is always specified. 
                    respond_status_code = test_response_body.get("respond_status_code", body.get("response_code"))
                    respond_body = test_response_body.get("respond_body", body.get("response_body"))
                    print(f'{datetime.now().strftime("%H:%M:%S.%f")} callback for {flow.request.url} returning custom response: HTTP {respond_status_code} {json.dumps(respond_body)}')
                    flow.response = Response.make(
                        respond_status_code, json.dumps(respond_body),
                        headers={
                            "MITM-Proxy": "yes", # so we don't reprocess this
                            "Content-Type": "application/json",
                        })
        except Exception as error:
            print(f"ERR: callback for {flow.request.url} returned {error}")
            print(f"ERR: callback, provided request body was {body}")
