import logging, os
from mitmproxy import ctx
from mitmproxy.addons import asgiapp
from flask import Flask, request

from status_code import StatusCode

app = Flask("mitmoptset")

@app.route("/", methods=["POST"])
def set_filters() -> str:
    body = request.json
    filters = body.get("filters", {})
    print(f"setting filters {filters}")
    for k, v in filters:
        ctx.options[k] = v
    return {}

addons = [
    asgiapp.WSGIApp(app, "mitm.local", 80), # requests to this host will be routed to the flask app
    StatusCode(),
]
