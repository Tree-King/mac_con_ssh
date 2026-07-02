FROM debian:12-slim

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
       openssh-server libpam-google-authenticator python3 ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -s /bin/bash tunnel \
    && echo 'tunnel:password' | chpasswd \
    && mkdir -p /run/sshd

# Test secret: JBSWY3DPEHPK3PXP. This fixture is for local testing only.
RUN printf 'JBSWY3DPEHPK3PXP\n" RATE_LIMIT 3 30\n" WINDOW_SIZE 3\n" DISALLOW_REUSE\n' > /home/tunnel/.google_authenticator \
    && chown tunnel:tunnel /home/tunnel/.google_authenticator \
    && chmod 600 /home/tunnel/.google_authenticator

RUN sed -i 's/^#\?PasswordAuthentication .*/PasswordAuthentication yes/' /etc/ssh/sshd_config \
    && sed -i 's/^#\?KbdInteractiveAuthentication .*/KbdInteractiveAuthentication yes/' /etc/ssh/sshd_config \
    && sed -i 's/^#\?UsePAM .*/UsePAM yes/' /etc/ssh/sshd_config \
    && printf '\nauth required pam_google_authenticator.so nullok\n' >> /etc/pam.d/sshd

EXPOSE 22 8000
CMD sh -c 'cd /home/tunnel && python3 -m http.server 8000 >/tmp/http.log 2>&1 & exec /usr/sbin/sshd -D -e'
