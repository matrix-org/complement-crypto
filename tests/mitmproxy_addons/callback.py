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

# Callback will intercept a response and send a POST request to the provided callback_url, with
# the following JSON object. Supports filters: https://docs.mitmproxy.org/stable/concepts-filters/
# {
#   method: "GET|PUT|...",
#   access_token: "syt_11...",
#   url: "http://hs1/_matrix/client/...",
#   request_body: { some json object or null if no body },
#   response_body: { some json object },
#   response_code: 200,
# }
# Currently this is a read-only callback. The response cannot be modified, but side-effects can be
# taken. For example, tests may wish to terminate a client prior to the delivery of a response but
# after the server has processed the request, or the test may wish to use the response as a
# synchronisation point for a Waiter.
class Callback:
    def __init__(self):
        self.reset()
        self.matchall = flowfilter.parse(".")
        self.filter: Optional[flowfilter.TFilter] = self.matchall

    def reset(self):
        self.config = {
            "callback_url": "",
            "filter": None,
        }

    def load(self, loader):
        loader.add_option(
            name="callback",
            typespec=dict,
            default={"callback_url": "", "filter": None},
            help="Change the callback url, with an optional filter",
        )

    def configure(self, updates):
        if "callback" not in updates:
            self.reset()
            return
        if ctx.options.callback is None or ctx.options.callback["callback_url"] == "":
            self.reset()
            return
        self.config = ctx.options.callback
        new_filter = self.config.get('filter', None)
        print(f"callback will hit {self.config['callback_url']} filter={new_filter}")
        if new_filter:
            self.filter = flowfilter.parse(new_filter)
        else:
            self.filter = self.matchall

    async def response(self, flow):
        # always ignore the controller
        if flow.request.pretty_host == MITM_DOMAIN_NAME:
            return
        if self.config["callback_url"] == "":
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
            print(f'{datetime.now().strftime("%H:%M:%S.%f")} hitting callback for {flow.request.url}')
            callback_body = {
                "method": flow.request.method,
                "access_token": flow.request.headers.get("Authorization", "").removeprefix("Bearer "),
                "url": flow.request.url,
                "response_code": flow.response.status_code,
                "request_body": req_body,
                "response_body": res_body,
            }
            try:
                # use asyncio so we don't block other unrelated requests from being processed
                async with aiohttp.request(
                    method="POST",url=self.config["callback_url"], timeout=aiohttp.ClientTimeout(total=10),
                    headers={"Content-Type": "application/json"},
                    json=callback_body) as response:
                    print(f'{datetime.now().strftime("%H:%M:%S.%f")} callback for {flow.request.url} returned HTTP {response.status}')
                    test_response_body = await response.json()
                    # if the response includes some keys then we are modifying the response on a per-key basis.
                    if len(test_response_body) > 0:
                        respond_status_code = test_response_body.get("respond_status_code", flow.response.status_code)
                        respond_body = test_response_body.get("respond_body", res_body)
                        flow.response = Response.make(
                            respond_status_code, json.dumps(respond_body),
                            headers={
                                "MITM-Proxy": "yes", # so we don't reprocess this
                                "Content-Type": "application/json",
                            })

            except Exception as error:
                print(f"ERR: callback for {flow.request.url} returned {error}")
                print(f"ERR: callback, provided request body was {callback_body}")
