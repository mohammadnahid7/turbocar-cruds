FROM golang:1.23.4 AS builder

WORKDIR /app

COPY . .

RUN go mod download

# Note: .env file is not needed - Railway uses environment variables directly
# The app will use env vars from Railway, godotenv.Load will just log if .env is missing

RUN CGO_ENABLED=0 GOOS=linux go build -C ./cmd -a -installsuffix cgo -o ./../myapp .

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/myapp .

# Copy required files and directories
# Note: firebase directory doesn't exist in cruds-main, so we skip it
COPY --from=builder /app/casbin/ ./casbin/
COPY --from=builder /app/doc/ ./doc/
COPY --from=builder /app/app.log ./

# Note: .env file is not copied - Railway uses environment variables directly

EXPOSE 8090

CMD ["./myapp"]