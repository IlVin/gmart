# Используем абсолютно пустой образ
FROM scratch

# Копируем наш предварительно собранный статичный бинарник
COPY ./PRAYER /PRAYER
COPY ./cmd/gophermart/gophermart /gophermart
COPY ./.env /.env

# Если сервис использует SSL (https), могут понадобиться сертификаты
# Их можно скопировать из хостовой системы, если нужно:
# COPY /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Экспонируем порт
EXPOSE 8080

# Запуск
ENTRYPOINT ["/gophermart"]
