FROM golang:1.24 
# RUN go install github.com/go-task/task/v3/cmd/task@latest
RUN /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
RUN brew install helix
RUN brew install yaml-language-server

WORKDIR /usr/src/app

COPY . .
RUN go install ./cmd/taskmaster

CMD ["bash", "-"]
