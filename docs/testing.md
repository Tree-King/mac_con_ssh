# Local Linux container test

Build and run the test OpenSSH + PAM TOTP container:

```bash
docker build -t ssh-tunnel-totp-test ./test/sshd-totp
docker run --rm --name ssh-tunnel-totp-test -p 2222:22 ssh-tunnel-totp-test
```

In another terminal, build and run the client:

```bash
go build -o ssh-tunnel ./cmd/ssh-tunnel
./ssh-tunnel check local-test --config ./test/sshd-totp/config.yaml
./ssh-tunnel run local-test --config ./test/sshd-totp/config.yaml
```

Verify the local forward:

```bash
curl http://127.0.0.1:8080
```

Reconnect test:

```bash
docker exec ssh-tunnel-totp-test sh -c 'pkill sshd; /usr/sbin/sshd'
```

The client should log that the session ended, reconnect, and continue serving the local forward.

Failure tests:

- Change `totp_seed` and confirm authentication fails with guidance to check seed and time.
- Bind `127.0.0.1:8080` with another process and confirm `check` reports the occupied local port.
- Change `remote_port` to a closed port and confirm per-connection forwarding logs show the remote dial failure.
