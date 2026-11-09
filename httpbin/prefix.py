import os
import io

from httpbin.core import app
from flask import request, make_response, abort

# --- Prefix middleware for subpath deployments ---

HTTPBIN_PREFIX = os.environ.get("HTTPBIN_PREFIX", "").rstrip("/")


class PrefixMiddleware:
    """WSGI middleware for serving httpbin behind a reverse proxy at a subpath."""

    def __init__(self, wsgi_app, prefix):
        self.wsgi_app = wsgi_app
        self.prefix = prefix

    def __call__(self, environ, start_response):
        path_info = environ.get("PATH_INFO", "")
        if not path_info.startswith(self.prefix):
            start_response("404 Not Found", [("Content-Type", "text/plain")])
            return [b"Not Found"]
        environ["SCRIPT_NAME"] = self.prefix
        environ["PATH_INFO"] = path_info[len(self.prefix) :] or "/"
        return self.wsgi_app(environ, start_response)


if HTTPBIN_PREFIX:
    app.config["APPLICATION_ROOT"] = HTTPBIN_PREFIX
    app.wsgi_app = PrefixMiddleware(app.wsgi_app, HTTPBIN_PREFIX)


# --- File store/retrieve endpoints ---

DATA_DIR = os.environ.get("DATA_DIR", "/tmp")


def _safe_path(filename):
    """Resolve file path and ensure it stays within DATA_DIR."""
    base = os.path.realpath(DATA_DIR)
    target = os.path.realpath(os.path.join(base, filename))
    if not target.startswith(base + os.sep) and target != base:
        abort(400, "Invalid filename")
    return target


@app.route("/files/<string:filename>", methods=["POST", "PUT"])
def store(filename):
    """Store a file."""
    outpath = _safe_path(filename)
    with io.open(outpath, "wb") as f:
        f.write(request.get_data())
    return make_response("", 200)


@app.route("/files/<string:filename>", methods=["GET"])
def retrieve(filename):
    """Retrieve a file."""
    readpath = _safe_path(filename)
    if not os.path.exists(readpath):
        return make_response("File does not exist", 404)
    with io.open(readpath, "rb") as f:
        data = f.read()
    response = make_response(data)
    response.content_type = "application/octet-stream"
    return response
