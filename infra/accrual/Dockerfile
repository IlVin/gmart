# Используем абсолютно пустой образ
FROM alpine:latest

RUN apk add --no-cache gcompat
RUN apk add --no-cache curl

# Копируем наш предварительно собранный статичный бинарник
COPY ./accrual_linux_amd64 /accrual_linux_amd64

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/api/orders/583612753 || exit 1

# Экспонируем порт
EXPOSE 8088

# Запуск
ENTRYPOINT ["/accrual_linux_amd64", "-a", ":8080"]

