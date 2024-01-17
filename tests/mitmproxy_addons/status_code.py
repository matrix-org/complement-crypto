from typing import Optional

from mitmproxy import ctx, flowfilter
from mitmproxy.http import Response
from controller import MITM_DOMAIN_NAME

# StatusCode will intercept a response and return the provided status code in its place, with
# no response body. Supports filters: https://docs.mitmproxy.org/stable/concepts-filters/
class StatusCode:
    def __init__(self):
        self.reset()
        print(MITM_DOMAIN_NAME)
        self.matchall = flowfilter.parse(".")
        self.filter: Optional[flowfilter.TFilter] = self.matchall

    def reset(self):
        self.config = {
            "return_status": 0,
            "block_request": False,
            "filter": None,
        }

    def load(self, loader):
        loader.add_option(
            name="statuscode",
            typespec=dict,
            default={"return_status": 0, "filter": None, "block_request": False},
            help="Change the response status code, with an optional filter",
        )

    def configure(self, updates):
        if "statuscode" not in updates:
            self.reset()
            return
        if ctx.options.statuscode is None or ctx.options.statuscode["return_status"] == 0:
            self.reset()
            return
        self.config = ctx.options.statuscode
        new_filter = self.config.get('filter', None)
        print(f"statuscode will return HTTP {self.config['return_status']} filter={new_filter}")
        if new_filter:
            self.filter = flowfilter.parse(new_filter)
        else:
            self.filter = self.matchall

    def request(self, flow):
        # always ignore the controller
        if flow.request.pretty_host == MITM_DOMAIN_NAME:
            return
        if self.config["return_status"] == 0:
            return # ignore responses if we aren't told a code
        if self.config["block_request"] and flowfilter.match(self.filter, flow):
            print(f'statuscode: blocking request and sending back {self.config["return_status"]}')
            flow.response = Response.make(self.config["return_status"])

    def response(self, flow):
        # always ignore the controller
        if flow.request.pretty_host == MITM_DOMAIN_NAME:
            return
        if self.config["return_status"] == 0:
            return # ignore responses if we aren't told a code
        if flowfilter.match(self.filter, flow):
            flow.response = Response.make(self.config["return_status"])
