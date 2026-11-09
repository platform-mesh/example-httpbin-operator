FROM kong/httpbin:latest@sha256:a6ac46531193a8009ca7d83d6a2d76b3114a7ab77c73aec5b8007c92206621f3

# Add prefix middleware for subpath deployments (HTTPBIN_PREFIX env var)
# and custom /files endpoints
COPY prefix.py /httpbin/

CMD ["pipenv", "run", "gunicorn", "-b", "0.0.0.0:80", "prefix:app", "-k", "gevent"]
