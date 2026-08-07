import json

import aiohttp
import attrs

import mitmproxy
from mitmproxy import ctx, flowfilter
from mitmproxy.http import Response
from controller import MITM_DOMAIN_NAME
from datetime import datetime

# See README.md for information about this addon
class Callback:
    _config: "Config"

    def __init__(self):
        self.reset()

    def reset(self):
        self._config = Config.from_config_dict({})

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

        new_config = Config.from_config_dict(ctx.options.callback)
        print(f"callback config={new_config}")
        self._config = new_config

    async def request(self, flow):
        # always ignore the controller
        if flow.request.pretty_host == MITM_DOMAIN_NAME:
            return
        config = self._config
        if config.callback_request_url == "":
            return  # ignore requests if we aren't told a url
        if not flowfilter.match(config.filter, flow):
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
        await self.send_callback(flow, config.callback_request_url, callback_body)

    async def response(self, flow):
        # always ignore the controller
        if flow.request.pretty_host == MITM_DOMAIN_NAME:
            return
        config = self._config
        if config.callback_response_url == "":
            return  # ignore responses if we aren't told a url
        if flowfilter.match(config.filter, flow):
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
            await self.send_callback(flow, config.callback_response_url, callback_body)

    async def send_callback(self, flow: mitmproxy.http.HTTPFlow, url: str, body: dict):
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

                    response_headers = {
                       "MITM-Proxy": "yes", # so we don't reprocess this
                       "Content-Type": "application/json",
                    }

                    # If we're handling a response callback, copy the CORS headers from the original response
                    if flow.response is not None:
                        response_headers.update({k: v for k, v in flow.response.headers.items() if k.startswith("Access-Control")})

                    flow.response = Response.make(
                        respond_status_code, json.dumps(respond_body), headers=response_headers,
                    )
        except Exception as error:
            print(f"ERR: callback for {flow.request.url} returned {error}")
            print(f"ERR: callback, provided request body was {body}")


@attrs.define
class Config:
    callback_request_url: str
    callback_response_url: str
    filter: flowfilter.TFilter = attrs.field(
        # When stringifying a Config, use the pattern of the filter.
        repr=lambda v: repr(v.pattern),
    )

    @staticmethod
    def from_config_dict(config_dict: dict) -> "Config":
        """Parse the config dict sent over HTTP from the application into a new Config object."""
        callback_request_url = config_dict.get("callback_request_url", "")
        callback_response_url = config_dict.get("callback_response_url", "")

        filter_string = config_dict.get('filter')
        if isinstance(filter_string, str):
            parsed_filter = flowfilter.parse(filter_string)
        else:
            # match-all, if no filter supplied
            parsed_filter = flowfilter.parse(".")

        return Config(callback_request_url, callback_response_url, parsed_filter)
