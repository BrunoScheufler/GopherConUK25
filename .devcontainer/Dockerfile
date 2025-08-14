FROM golang:1.25

WORKDIR /workspace

COPY go.mod ./
RUN go mod download

COPY . .

CMD ["make", "framework"]