# Imagem de DESENVOLVIMENTO: hot reload com air. A imagem de produção fica em
# Dockerfile.prod; ela compila um binário estático e não instala ferramenta
# nenhuma de desenvolvimento.
FROM golang:1.26-alpine

WORKDIR /app

# Versão fixa: com @latest, uma release nova do air quebraria o `make run` sem
# nenhuma mudança neste repositório — e o build de ontem deixaria de ser o de hoje.
RUN go install github.com/air-verse/air@v1.67.4

COPY go.mod go.sum ./
RUN go mod download

COPY . .

EXPOSE 8080

CMD ["air", "-c", ".air.toml"]
