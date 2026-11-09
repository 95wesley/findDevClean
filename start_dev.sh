#!/usr/bin/env bash
# Sobe o findDevClean em Docker para desenvolvimento local.
#
# Uso:
#   ./start_dev.sh              sobe (constrói a imagem se ela não existir) e abre no navegador
#   ./start_dev.sh --rebuild    reconstrói a imagem antes de subir
#   ./start_dev.sh --porta 8080 publica em outra porta do host (padrão: 7777)
#   ./start_dev.sh --stop       para o container
set -euo pipefail

cd "$(dirname "$0")"

NAME=findclean-dev
IMAGE=finddevclean:latest
PORT=7777
REBUILD=0
STOP=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --rebuild) REBUILD=1 ;;
    --stop) STOP=1 ;;
    --porta) PORT="${2:?informe a porta}"; shift ;;
    -h|--help) sed -n '2,8p' "$0"; exit 0 ;;
    *) echo "opção desconhecida: $1" >&2; exit 2 ;;
  esac
  shift
done

if [[ $STOP -eq 1 ]]; then
  if docker rm -f "$NAME" >/dev/null 2>&1; then echo "parado."; else echo "nada rodando."; fi
  exit 0
fi

ensure_docker() {
  if docker info >/dev/null 2>&1; then return; fi
  if [[ "$(uname)" == "Darwin" ]]; then
    echo "Docker não está rodando. Abrindo o Docker Desktop..."
    open -a Docker || true
  fi
  echo -n "Aguardando o daemon do Docker"
  for _ in $(seq 1 60); do
    if docker info >/dev/null 2>&1; then echo " ok"; return; fi
    echo -n "."
    sleep 2
  done
  echo
  echo "O Docker não respondeu. Inicie-o manualmente e rode o script de novo." >&2
  exit 1
}
ensure_docker

if [[ $REBUILD -eq 1 ]] || ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
  echo "Construindo a imagem $IMAGE..."
  docker build -t "$IMAGE" .
fi

docker rm -f "$NAME" >/dev/null 2>&1 || true

echo "Montando seu diretório home no container (limpezas passam pela quarentena e podem ser desfeitas)."
# A porta do host fica só em 127.0.0.1. Dentro do container o app escuta em 0.0.0.0.
docker run -d --name "$NAME" \
  -p "127.0.0.1:${PORT}:7777" \
  --user "$(id -u):$(id -g)" \
  -v "$HOME:/home/nonroot" \
  "$IMAGE" --addr 0.0.0.0:7777 --public-port "$PORT" >/dev/null

URL=""
for _ in $(seq 1 30); do
  URL=$(docker logs "$NAME" 2>&1 | grep -o 'http://localhost:[0-9]*/?token=[0-9a-f]*' | head -1 || true)
  [[ -n "$URL" ]] && break
  sleep 1
done

if [[ -z "$URL" ]]; then
  echo "O container não iniciou. Logs:" >&2
  docker logs "$NAME" >&2 || true
  exit 1
fi

echo
echo "findDevClean rodando em: $URL"
echo "Parar: ./start_dev.sh --stop"

if [[ "$(uname)" == "Darwin" ]]; then
  open "$URL" || true
fi
