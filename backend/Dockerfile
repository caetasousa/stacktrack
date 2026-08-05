# Imagem de DESENVOLVIMENTO: hot reload com air. A imagem de produção
# (Dockerfile.prod) chega na fase 8 — ela compila um binário estático e não
# instala ferramenta nenhuma de desenvolvimento.
FROM golang:1.26-alpine

WORKDIR /app

RUN go install github.com/air-verse/air@latest

COPY go.mod go.sum ./
RUN go mod download

COPY . .

EXPOSE 8080

CMD ["air", "-c", ".air.toml"]
