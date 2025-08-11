FROM docker.n8n.io/n8nio/n8n
USER root
RUN apk --update add curl
USER node
