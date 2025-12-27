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
COPY --from=builder /app/firebase/wegugin-cars-notifications-firebase-adminsdk-fbsvc-0016bd1639.json ./firebase/
COPY --from=builder /app/casbin/model.conf ./casbin/
COPY --from=builder /app/casbin/policy.csv ./casbin/
COPY --from=builder /app/doc/swagger/index.html ./doc/swagger/
COPY --from=builder /app/doc/swagger/swagger_docs.swagger.json ./doc/swagger/
COPY --from=builder /app/app.log ./
# Note: .env file is not copied - Railway uses environment variables directly

EXPOSE 8090

CMD ["./myapp"]