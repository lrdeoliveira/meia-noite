# Como contribuir

Toda ajuda é bem-vinda: relato de defeito, ideia, documentação ou código.

## Antes de abrir um pull request

```bash
gofmt -l .        # não pode listar nada
go vet ./...
go test ./...
```

- Escreva em português (código, comentários e mensagens). É um projeto feito para quem fala português.
- Mudou comportamento? Atualize o README na mesma mudança.
- Toda trava de segurança tem teste. Se você mexer numa, o teste vem junto.
- Nada de dependência externa sem conversar antes: hoje o projeto usa só a biblioteca padrão do Go.

## O que não entra

- Opção que desliga as travas das CLIs (`--force`, `--yolo`, `--dangerously-*`).
- Porta de rede aberta para fora de `127.0.0.1`.
- Chave, token ou caminho pessoal em código, teste ou exemplo.

## Onde a ajuda faz mais diferença

Veja "O que falta" no README. Os três pontos que mais mudam o uso de verdade:

1. Usar no Linux ou no Windows e contar o que funcionou e o que quebrou.
2. Detector de palavra de chamada dedicado em português (hoje a chamada funciona transcrevendo tudo localmente).
3. Cancelamento de eco, para dar para interromper falando por cima.
4. Mais provedores de cérebro e de voz testados de verdade.

## Segurança

Achou uma falha? Não abra issue pública com os detalhes: use o "Report a vulnerability" na aba Security do repositório.
