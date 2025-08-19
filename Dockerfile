FROM golang:1.24 
# RUN go install github.com/go-task/task/v3/cmd/task@latest

WORKDIR /usr/src/app

COPY . .
RUN go install ./cmd/taskmaster

CMD ["bash", "-"]
