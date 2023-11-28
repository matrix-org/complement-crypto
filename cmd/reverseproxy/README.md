### Complement Reverse Proxy

This is a sidecar container which runs in the same network as the homeservers. All clients should be pointed to this sidecar, who will then reverse proxy to the correct homeserver. This sidecar exposes a "controller" HTTP API to manipulate client/server responses.

Rebuild: (from root of this repository)
```
docker build -t rp -f cmd/reverseproxy/Dockerfile .
```
Usage:
```
$ docker run --rm -e "REVERSE_PROXY_CONTROLLER_URL=http://somewhere-tests-are-listening" -e "REVERSE_PROXY_HOSTS=http://hs1,3000;http://hs2,3001" rp
2023/11/28 16:36:40 newComplementProxy on port 3000 : forwarding to http://hs1
2023/11/28 16:36:40 newComplementProxy on port 3001 : forwarding to http://hs2
2023/11/28 16:36:40 listening
```
Then tell clients to connect to the reverse proxy on the respective port.

This is handled for you by complement-crypto by default.