# findDevClean

[![Licença: MIT](https://img.shields.io/badge/licen%C3%A7a-MIT-green)](LICENSE)
[![Versão](https://img.shields.io/badge/vers%C3%A3o-0.1.0-blue)](https://github.com/95wesley/findDevClean/releases/latest)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black)

Libera espaço em disco removendo **caches e artefatos de build** de desenvolvimento
(`node_modules`, `target`, `__pycache__`, caches do npm, pip, Cargo, Gradle, Go, Xcode e Homebrew),
com **simulação por padrão** e **quarentena com desfazer**. Nada é apagado de imediato.

- **Um binário:** CLI, API local e interface web juntas, sem dependências em tempo de execução.
- **Local first:** nada sai da sua máquina. A API só escuta em `localhost`, com token por execução.
- **Regras declarativas:** cada ferramenta é uma regra em YAML embutida no binário.

## Sumário

- [Início rápido](#início-rápido)
- [Como funciona](#como-funciona)
- [Instalação](#instalação)
- [Uso pela linha de comando](#uso-pela-linha-de-comando)
- [Mapa de disco](#mapa-de-disco)
- [Regras de limpeza](#regras-de-limpeza)
- [Segurança](#segurança)
- [Limitações conhecidas](#limitações-conhecidas)
- [Desenvolvimento](#desenvolvimento)
- [Contribuição](#contribuição)
- [Licença](#licença)

## Início rápido

| Eu quero…                                      | Faça isto                                                                     |
| ---------------------------------------------- | ----------------------------------------------------------------------------- |
| Usar no Mac como um app                        | Baixe o `.zip` para macOS na [release](https://github.com/95wesley/findDevClean/releases/latest) ou rode `make install-app` |
| Usar no Windows                                | Baixe o `.zip` para Windows e execute `findDevClean.exe`                       |
| Usar no Linux                                  | Baixe o `.tar.gz` para Linux, extraia e execute `./findDevClean`              |
| Rodar com Docker                               | `./start_dev.sh` (veja [Instalação](#instalação))                             |
| Ver o que pode ser liberado, sem apagar nada   | `findDevClean scan` ou use a interface e confira a lista                      |
| Entender o que ocupa espaço                    | Abra o **Mapa de disco** na interface                                         |

## Como funciona

O processo tem quatro passos, e cada um pode ser feito antes do próximo:

1. **Escanear.** O programa percorre as pastas de projeto e mede os caches conhecidos.
   Mostra o total que pode ser liberado, a quantidade de itens e o nível de risco de cada um
   (`seguro`, `regenerável` ou `precaução`). A barra de progresso mostra a fase atual.
2. **Simular.** Ao escolher os itens, a opção **Simular** é a padrão. Ela mostra o que seria movido,
   sem tocar em nada.
3. **Mover para a quarentena.** Ao confirmar, cada pasta vai para `~/.findDevClean/quarantine`,
   junto com um manifesto que registra de onde ela veio. Nada é apagado.
4. **Desfazer.** Na lista de lotes da quarentena, **Desfazer** devolve os itens ao lugar original.
   Se o destino já estiver ocupado, o item fica na quarentena e é reportado.

## Instalação

| Sistema                  | Como instalar                                                                                              |
| ------------------------ | ---------------------------------------------------------------------------------------------------------- |
| macOS (Apple Silicon)    | Baixe `findDevClean-<versão>-macos-arm64.zip` na [release](https://github.com/95wesley/findDevClean/releases/latest), descompacte e abra o app. Se o macOS bloquear, use botão direito → **Abrir**. |
| Windows (x64)            | Baixe `findDevClean-<versão>-windows-amd64.zip`, descompacte e execute `findDevClean.exe`. O Windows pode mostrar o aviso do SmartScreen, porque o executável não é assinado. |
| Linux (x64 ou ARM64)     | Baixe o `.tar.gz` correspondente, extraia com `tar -xzf` e execute `./findDevClean`.                       |
| Docker                   | `./start_dev.sh` (ou `make docker` e depois `docker run`). Veja abaixo.                                    |
| A partir do código       | `make build`, que gera `bin/findDevClean` com a interface embutida.                                        |

Os arquivos `SHA256SUMS.txt` da release permitem conferir a integridade de cada pacote.

**Sem argumentos**, o binário sobe o servidor em uma porta livre e abre o navegador. No macOS, o app
faz o mesmo, mas em uma janela própria.

### Requisitos para compilar

- Go **1.26** ou superior
- Node.js **22** e npm (a interface é compilada antes do binário)
- No macOS, para gerar o app: `swiftc`, `sips` e `iconutil` (vêm com as ferramentas de linha de comando do Xcode; se faltarem, rode `xcode-select --install`)
- `make`, para os atalhos do Makefile

### Docker

```sh
./start_dev.sh            # sobe o container e abre a interface
./start_dev.sh --stop     # para o container
./start_dev.sh --rebuild  # reconstrói a imagem depois de mudar o código
```

O script inicia o Docker Desktop no macOS se ele estiver fechado e publica a porta só em `127.0.0.1`.
**Atenção:** o diretório home é montado no container, então as limpezas afetam os arquivos reais.
Por isso tudo passa pela quarentena e pode ser desfeito.

Para publicar em outra porta do host (ex.: `7788`), passe as flags depois da imagem. O `ENTRYPOINT`
já executa `serve`, então só as flags vão no comando:

```sh
docker run --rm -p 127.0.0.1:7788:7777 --user "$(id -u):$(id -g)" \
  -v "$HOME:/home/nonroot" finddevclean:latest --addr 0.0.0.0:7777 --public-port 7788
```

`--public-port` diz à API qual porta o navegador usa no header `Host`.

## Uso pela linha de comando

```sh
findDevClean scan                                    # lista o que pode ser liberado
findDevClean clean ~/dev/app/node_modules            # simula (padrão)
findDevClean clean --apply ~/dev/app/node_modules    # move para a quarentena
findDevClean batches                                 # lista os lotes da quarentena
findDevClean undo <ID>                               # restaura um lote
findDevClean serve                                   # sobe o servidor e mostra a URL com token
findDevClean serve --open                            # igual, e abre o navegador
```

Opções úteis: `scan --root DIR` (pode repetir), `scan --depth N`, `scan --json`.

## Mapa de disco

O mapa mostra a pasta home como um mapa de árvore: cada bloco é proporcional ao espaço que a pasta ocupa.

- Clique em uma pasta para entrar nela. O caminho de navegação fica no topo, e **Início** volta à home.
- Mostra **dois níveis**. Pastas pequenas são agrupadas em **outros**.
- O tamanho é o **espaço realmente ocupado** no disco, não o tamanho lógico do arquivo.
- Pastas grandes levam tempo para medir. No macOS, rodando nativamente, a home inteira leva cerca de um minuto.
  Rodando dentro do Docker no macOS, o tempo é bem maior, porque o compartilhamento de arquivos é lento.
- Links simbólicos não são seguidos, então nunca contam duas vezes.

## Regras de limpeza

As regras ficam em `internal/rules/default.yaml`. Há dois tipos:

```yaml
# Regra de projeto: casa um diretório pelo nome, se um arquivo marcador existir no pai.
- id: node-modules
  name: node_modules
  category: javascript
  risk: regenerable
  dir: node_modules
  markers: [package.json]
  description: Dependências do npm, yarn ou pnpm. Reinstale com o gerenciador do projeto.

# Regra global: caminhos fixos dentro da home, por sistema.
- id: npm-cache
  name: Cache do npm
  category: javascript
  risk: regenerable
  paths:
    darwin: [.npm/_cacache]
    linux: [.npm/_cacache]
    windows: [AppData/Local/npm-cache]
  description: Tarballs baixados pelo npm.
```

Para adicionar uma ferramenta, basta uma regra nova. O `risk` pode ser `safe`, `regenerable` ou `caution`.

## Segurança

- A API só aceita `Host` `localhost` ou `127.0.0.1`, o que bloqueia ataques de DNS rebinding.
- Valida o header `Origin` quando ele existe.
- Exige um token aleatório gerado a cada execução.
- O `clean` só move diretórios que casam com uma regra e estão dentro da pasta home.
  Links simbólicos, a própria home e a quarentena são recusados.
- Não há telemetria nem conexão de saída. O programa só escuta em loopback.

## Limitações conhecidas

- **Windows:** o pacote foi compilado e os testes foram compilados, mas o `.exe` ainda não foi executado numa máquina Windows real.
- Mover uma pasta para a quarentena falha se a pasta estiver em outro disco ou volume que o da home. Nesse caso o item aparece como **falhou**.
- Arquivos em uso ou bloqueados não podem ser movidos. Eles aparecem como **falhou**, sem nenhum dano.
- Caminhos de cache globais dependem da plataforma. Algumas ferramentas usam locais diferentes em cada sistema.
- O mapa de disco mede a home inteira; não há limite de tamanho por pasta.

## Desenvolvimento

```sh
make test     # testes Go
make vet     # análise estática
make build   # recompila a interface e o binário
make app     # gera o app do macOS (só em um Mac)
make release # gera os pacotes de Windows, Linux e macOS em dist/release
```

O código fica em `cmd/` (linha de comando), `internal/` (scanner, regras, cleaner, servidor e mapa de disco),
`web/` (interface em React e Vite) e `scripts/` (geração do app e dos pacotes).

## Contribuição

- Trabalhe na branch **`dev`** e abra um pull request para a **`main`**.
- Antes de enviar, rode `make test` e `make vet`.
- Para uma regra nova, inclua um teste em `internal/scanner` ou `internal/cleaner` que mostre o que ela casa e o que ela não casa.

## Licença

MIT. Veja [LICENSE](LICENSE).
