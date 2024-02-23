from typing import Optional
import json

from mitmproxy import ctx, flowfilter
from mitmproxy.http import Response
from controller import MITM_DOMAIN_NAME
from urllib.request import urlopen, Request
from urllib.error import HTTPError, URLError

# Callback will intercept a response and send a POST request to the provided callback_url, with
# the following JSON object. Supports filters: https://docs.mitmproxy.org/stable/concepts-filters/
# {
#   method: "GET|PUT|...",
#   access_token: "syt_11...",
#   url: "http://hs1/_matrix/client/...",
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

    def response(self, flow):
        # always ignore the controller
        if flow.request.pretty_host == MITM_DOMAIN_NAME:
            return
        if self.config["callback_url"] == "":
            return # ignore responses if we aren't told a url
        if flowfilter.match(self.filter, flow):
            data = json.dumps({
                "method": flow.request.method,
                "access_token": flow.request.headers.get("Authorization", "").removeprefix("Bearer "),
                "url": flow.request.url,
                "response_code": flow.response.status_code,
                "request_body": flow.request.json(),
            })
            request = Request(
                self.config["callback_url"],
                headers={"Content-Type": "application/json"},
                data=data.encode("utf-8"),
            )
            try:
                with urlopen(request, timeout=10) as response:
                    print(f"callback returned HTTP {response.status}")
                    return response.read(), response
            except HTTPError as error:
                print(f"ERR: callback returned {error.status} {error.reason}")
            except URLError as error:
                print(f"ERR: callback returned {error.reason}")
            except TimeoutError:
                print(f"ERR: callback request timed out")
