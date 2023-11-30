import random
from mitmproxy import ctx
from flask import Flask, request, make_response

MITM_DOMAIN_NAME = "mitm.local"
app = Flask("mitmoptset")

prev_options = {
    "lock_id": "",
    "options": {},
}

# Set options on mitmproxy. See https://docs.mitmproxy.org/stable/concepts-options/
# This is intended to be used exclusively for our addons in this package, but nothing
# stops tests from enabling/disabling/tweaking other mitmproxy options.
# POST /options/lock
# {
#   "options": {
#     "body_size_limit": "3m",
#   }
# }
# HTTP/1.1 200 OK
# {
#   "reset_id": "some_opaque_string"
# }
# Calling this endpoint locks the proxy from further option modification until /options/unlock
# is called. This ensures that tests can't forget to reset options when they are done with them.
@app.route("/options/lock", methods=["POST"])
def lock_options():
    if prev_options["lock_id"] != "":
        return make_response(("options already locked, did you forget to unlock?", 400))
    body = request.json
    options = body.get("options", {})
    prev_options["lock_id"] = bytes.hex(random.randbytes(8))
    for k, v in ctx.options.items():
        if k in options:
            prev_options["options"][k] = v.current()
    print(f"locking options {options}")
    ctx.options.update(**options)
    return {
        "reset_id": prev_options["lock_id"]
    }

# Unlock previously set options on mitmproxy. Must be called after a call to POST /options/lock
# to allow further option modifications.
# POST /options/unlock
# {
#   "reset_id": "some_opaque_string"
# }
@app.route("/options/unlock", methods=["POST"])
def unlock_options() -> str:
    body = request.json
    reset_id = body.get("reset_id", "")
    if prev_options["lock_id"] == "":
        return make_response(("options were not locked, mismatched lock/unlock calls", 400))
    if prev_options["lock_id"] != reset_id:
        return make_response(("refusing to unlock, wrong id supplied", 400))
    print(f"unlocking options back to {prev_options['options']}")
    ctx.options.update(**prev_options["options"])
    # apply AFTER update so if we fail to reset them back we won't unlock, indicating a problem.
    prev_options["lock_id"] = ""
    prev_options["options"] = {}
    return {}

# Creates a filter which can then be passed to options
# POST /create_filter
# {
#   "hs": "hs1|hs2|*|"      empty disables filter, * matches all hses, else HS domain
#   "user": "@alice:hs1|*|" empty disables filter, * matches all users, else user ID
#   "device": "FOO|*|"      empty disables filter, * matches all devices, else device ID
# }
# HTTP/1.1 200 OK
# {
#   "filter_id": "some_opaque_string"
# }
@app.route("/create_filter", methods=["POST"])
def create_filter() -> str:
    body = request.json
    filter = body.get("filter", {})
    print(f"creating filter {filter}")
    hs_filter = filter.get("hs", "")
    user_filter = filter.get("user", "")
    device_filter = filter.get("device", "")
    ctx.options.update(**options)
    return {}