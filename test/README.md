# Tests

English | [中文](#中文)

Four suites, four origins. Everything runs offline with `go test ./...` from the repository root; the only database touched is an in-memory sqlite.

- **`readme/`** — the code examples of the root README, kept verbatim. The hero and execution snippets round-trip real data through in-memory sqlite; the compile-only and qdata snippets assert the exact SQL printed on the front page. When a README example changes, the test changes with it, so the documented output cannot drift from the library.
- **`tutorial/`** — the examples of the `docs/tutorial` pages (English and Chinese), under the same rule as `readme/`: the example changes here first, then in the doc.
- **`goqu/`** — scenario cases informed by goqu's dialect-assertion tests: each case pairs build code with the expected SQL text and argument sequence, asserted at the `compiler.Compile` seam.
- **`go-sqlbuilder/`** — the `*_test.go` scenarios of [go-sqlbuilder](https://github.com/huandu/go-sqlbuilder) migrated onto the sqlk builder: 23 test functions covering the five dialects sqlk supports. Where sqlk's output differs from go-sqlbuilder's, the case comment states the difference instead of hiding it.

The combination is the point. `readme` and `tutorial` keep the project's own documentation executable; `goqu` and `go-sqlbuilder` hold sqlk's SQL against expectations two other builders have already shipped and maintained. A change that contradicts either the documented output or a ported scenario fails here, before it reaches a database.

---

## 中文

四套测试,四个来源。仓库根目录 `go test ./...` 即可全部离线运行,唯一用到的数据库是内存 sqlite。

- **`readme/`** —— 根 README 的代码示例逐字留存:主示例与执行示例在内存 sqlite 上做真实数据往返,纯编译示例与 qdata 示例断言首页印出的 SQL 原文。README 示例变更时先改这里的测试再改文档,文档输出就不会与库漂移。
- **`tutorial/`** —— `docs/tutorial` 中英文教程页的示例,与 `readme/` 同一条规则:先在测试里改,再改文档。
- **`goqu/`** —— 受 goqu 方言断言测试启发的场景用例:每个用例把构建代码与期望的 SQL 文本、参数序列配对,在 `compiler.Compile` 主接缝上断言。
- **`go-sqlbuilder/`** —— [go-sqlbuilder](https://github.com/huandu/go-sqlbuilder) 的 `*_test.go` 场景移植到 sqlk 构建器:23 个测试函数,覆盖 sqlk 支持的五种方言。sqlk 输出与 go-sqlbuilder 不同之处,用例注释里写明差异而非掩饰。

组合本身就是目的:`readme` 与 `tutorial` 让项目自己的文档保持可执行;`goqu` 与 `go-sqlbuilder` 把 sqlk 的 SQL 对照另外两个已发布并长期维护的构建器的预期。一个改动只要与文档输出或任一移植场景相矛盾,就会在这里失败,而不是等到真数据库上才暴露。
