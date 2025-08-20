FROM golang:1.24 
# RUN go install github.com/go-task/task/v3/cmd/task@latest

RUN curl -LO https://github.com/sharkdp/bat/releases/download/v0.25.0/bat_0.25.0_amd64.deb
RUN dpkg -i bat_0.25.0_amd64.deb

RUN curl -LO https://github.com/helix-editor/helix/releases/download/25.07.1/helix_25.7.1-1_amd64.deb
RUN dpkg -i helix_25.7.1-1_amd64.deb


WORKDIR /usr/src/app

COPY . .
RUN go install ./cmd/taskmaster
RUN go install ./cmd/taskmasterctl
RUN go install ./cmd/taskmasterd

CMD ["bash", "-"]
