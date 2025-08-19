FROM ubuntu:questing

WORKDIR /usr/src/app

COPY taskmaster /usr/local/bin/app

COPY config.yaml .

CMD ["app", "-config=config.yaml"]
