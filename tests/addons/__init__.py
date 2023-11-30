import logging, os
from mitmproxy import ctx
from mitmproxy.addons import asgiapp
from flask import Flask, request

from status_code import StatusCode

app = Flask("mitmoptset")

@app.route("/", methods=["POST"])
def set_options() -> str:
    body = request.json
    options = body.get("options", {})
    print(f"setting options {options}")
    for k, v in options:
        ctx.options[k] = v
    return {}

addons = [
    asgiapp.WSGIApp(app, "mitm.local", 80), # requests to this host will be routed to the flask app
    StatusCode(),
]
