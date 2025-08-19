FROM ubuntu:questing

WORKDIR /usr/src/app

COPY ./bin/linux/taskmaster /usr/local/bin/taskmaster
COPY ./bin/linux/taskmasterd /usr/local/bin/taskmasterd
COPY ./bin/linux/taskmasterctl /usr/local/bin/taskmasterclt

COPY config.yaml ~/.config/taskmaster/config.yaml

CMD ["bash" "-"]
