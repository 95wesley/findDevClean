# findDevClean

Libera espaço de caches e artefatos de build de desenvolvimento
(`node_modules`, `target`, `__pycache__`, caches do npm, pip, Cargo, Gradle, Go, Xcode e Homebrew).

- **Seguro por padrão:** a simulação é o padrão. Nada é apagado de imediato: os itens vão
  para uma quarentena (`~/.findDevClean/quarantine`) e podem ser restaurados com `undo`.
- **Um binário:** CLI, API local e interface web, sem dependências em tempo de execução.
- **Regras declarativas:** as regras ficam em `internal/rules/default.yaml`.

## Uso rápido

```sh
make build                       # gera bin/findDevClean (com a UI embutida)
bin/findDevClean scan            # lista o que pode ser liberado
bin/findDevClean clean ~/dev/app/node_modules          # simula
bin/findDevClean clean --apply ~/dev/app/node_modules  # move para a quarentena
bin/findDevClean batches         # lista lotes da quarentena
bin/findDevClean undo <ID>       # restaura um lote
bin/findDevClean serve           # abre a UI em http://localhost:7777/?token=...
```

## Início rápido (dev, com Docker)

```sh
./start_dev.sh            # sobe o container e abre a UI no navegador
./start_dev.sh --stop     # para o container
./start_dev.sh --rebuild  # reconstrói a imagem depois de mudar o código
```

O script inicia o Docker Desktop no macOS se ele estiver fechado e publica a
porta só em `127.0.0.1`. O seu diretório home é montado no container, então
as limpezas afetam os arquivos reais, sempre pela quarentena e com `undo`.

## App para Mac

Gera um `findDevClean.app` nativo: abre em uma janela própria, sem navegador.
O app inicia o servidor local embutido, em uma porta livre, e o encerra ao sair.
Não precisa de Docker.

```sh
make install-app          # gera e instala em ~/Applications/findDevClean.app
# ou, sem instalar:
make app                  # gera dist/findDevClean.app
open dist/findDevClean.app
```

Requisitos: macOS 13+, Go, Node/npm e as ferramentas do Xcode (`swiftc`, `sips`, `iconutil`).
Se faltar o compilador, rode `xcode-select --install`.

- A janela é feita com WebKit e o código fica em `scripts/mac-app/main.swift`.
- Cada execução usa uma porta livre, então o app não conflita com o Docker (porta 7777).
- Ao fechar o app (⌘Q), o servidor também para.
- O log do servidor fica em `~/Library/Logs/findDevClean.log`.
- Para remover: apague `~/Applications/findDevClean.app`.

O app é gerado localmente com assinatura ad-hoc, então o macOS normalmente não bloqueia a abertura.
Se bloquear, use o botão direito → **Abrir**.

## Docker

```sh
make docker
docker run --rm -p 127.0.0.1:7777:7777 --user "$(id -u):$(id -g)" \
  -v "$HOME:/home/nonroot" finddevclean:latest
```

A porta é publicada só em `127.0.0.1`. A URL com o token aparece no log.

Para publicar em outra porta do host (ex.: `7788`), passe as flags ao container. O
`ENTRYPOINT` já executa `serve`, então só as flags vão depois da imagem:

```sh
docker run --rm -p 127.0.0.1:7788:7777 --user "$(id -u):$(id -g)" \
  -v "$HOME:/home/nonroot" finddevclean:latest --addr 0.0.0.0:7777 --public-port 7788
```

`--public-port` diz à API qual porta o navegador usa no header `Host`.

## Segurança da API local

- Só aceita `Host` `localhost` ou `127.0.0.1` (bloqueia DNS rebinding).
- Valida `Origin` quando presente.
- Exige um token aleatório por execução.
- `clean` só move diretórios que casam com uma regra e estão dentro da pasta home.
  Links simbólicos, a própria home e a quarentena são recusados.

## Desenvolvimento

```sh
make test     # testes Go
make vet
make build    # recompila a UI e o binário; a UI é embutida, então não há servidor de dev separado
```

## Licença

MIT. Veja [LICENSE](LICENSE).
