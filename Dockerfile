FROM docker.m.daocloud.io/library/golang:1.26.3-bookworm

ENV GOTOOLCHAIN=local
ENV CGO_ENABLED=0
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOTOOLCHAIN=local go build -o /app/echotrack ./cmd/echotrack

EXPOSE 8080
ENTRYPOINT ["/app/echotrack"]
CMD ["--smoke-test"]
