# Etapa de build
FROM golang:1.22-alpine AS builder

# Definir o diretório de trabalho dentro do contêiner
WORKDIR /app

# Copiar o código fonte para o contêiner
COPY . .

# Baixar as dependências
RUN go mod download

# Compilar a aplicação
RUN go build -o main .

# Etapa final
FROM alpine:latest

# Definir o diretório de trabalho na imagem final
WORKDIR /app

# Copiar o binário da etapa de build para a imagem final
COPY --from=builder /app/main .

# Expor a porta que a aplicação irá utilizar
EXPOSE 8080

# Verifique se o binário existe e tem permissões de execução
RUN chmod +x ./main

# Comando para rodar a aplicação
CMD ["./main"]
