# manatcp

A simple framework for tcp clients interacting with command/response servers
which may also push messages.

See the [API docs][doc] for usage. There's also a basic example in the
[example][example] folder.

# Installation

If you're using [goat][goat]:

```yaml
    - loc: https://github.com/mediocregopher/manatcp.git
      type: git
      ref: v0.2.1
      path: github.com/mediocregopher/manatcp
```

Otherwise:

```
go get github.com/mediocregopher/manatcp
```

[doc]: http://godoc.org/github.com/mediocregopher/manatcp
[example]: /example
[goat]: https://github.com/mediocregopher/goat
