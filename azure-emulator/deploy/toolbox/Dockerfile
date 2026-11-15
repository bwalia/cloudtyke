# Toolbox: OpenTofu + Terraform + kubectl/jq/curl, trusting the CloudTyke CA.
# Runs on the emulator's Docker network so names resolve via the DNS sidecar
# and Go-based tools honour SSL_CERT_FILE (they ignore it on macOS hosts).
FROM alpine:3.21
ARG TOFU_VERSION=1.13.9
ARG TERRAFORM_VERSION=1.17.2
ARG KUBECTL_VERSION=1.33.5
ARG TARGETARCH
RUN apk add --no-cache bash curl jq ca-certificates git unzip make python3 \
 && curl -fsSL -o /tmp/tofu.zip "https://github.com/opentofu/opentofu/releases/download/v${TOFU_VERSION}/tofu_${TOFU_VERSION}_linux_${TARGETARCH}.zip" \
 && unzip -o /tmp/tofu.zip tofu -d /usr/local/bin \
 && curl -fsSL -o /tmp/tf.zip "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_${TARGETARCH}.zip" \
 && unzip -o /tmp/tf.zip terraform -d /usr/local/bin \
 && curl -fsSL -o /usr/local/bin/kubectl "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" \
 && chmod +x /usr/local/bin/kubectl \
 && rm -f /tmp/*.zip
COPY entrypoint.sh /usr/local/bin/cloudtyke-toolbox
ENTRYPOINT ["/usr/local/bin/cloudtyke-toolbox"]
CMD ["bash"]
