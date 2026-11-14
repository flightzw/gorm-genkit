# gorm-genkit

参考 [GORM gentool](https://github.com/go-gorm/gen/blob/master/tools/gentool/gentool.go)
实现的数据库代码生成工具，支持 MySQL、PostgreSQL、SQLite、SQL Server 和 ClickHouse。
可以生成模型、查询代码及查询测试，并通过 YAML 配置数据库字段类型到 Go 类型的映射。

```bash
go build -o genkit ./cmd/genkit
./genkit -c gen.yml
```

也可以只使用命令行参数：

```bash
go run ./cmd/genkit -db sqlite -dsn ./example.db -tables users,orders -outPath ./dao/query
go run ./cmd/genkit -h
```

指定 `-c` 时，配置文件优先，其他命令行参数不参与合并；未配置的字段使用默认值。
所有相对路径（包括 SQLite DSN、输出目录和模板路径）均相对于运行命令时的工作目录。
配置文件使用上游的 `version` / `database` 结构；未知字段、重复键和无效类型配置会报错。

## 类型映射

`dataTypeMap` 和 `importPkgPaths` **仅支持通过配置文件加载**，不提供对应命令行参数。
完整示例见 [gen.yml](gen.yml)。

```yaml
version: "1"
database:
  db: mysql
  dsn: "root:password@tcp(127.0.0.1:3306)/example?parseTime=True"
  outPath: ./dao/query
  modelPkgName: model
  dataTypeMap:
    bigint: uint64
    decimal: decimal.Decimal
    json: datatypes.JSON
  importPkgPaths:
    - github.com/shopspring/decimal
    - gorm.io/datatypes
```

- 键是 GORM 驱动返回的 `ColumnType.DatabaseTypeName()`，区分大小写，需与驱动结果完全一致。它不是带长度等修饰的完整列定义，如使用 `varchar` 而非 `varchar(255)`。
- 值是生成字段使用的 Go 类型表达式，例如 `uint64`、`[]byte`、`*time.Time` 或 `decimal.Decimal`。未匹配的数据库类型继续使用 GORM Gen 默认规则。
- `importPkgPaths` 指定自定义类型的包路径；值中的包名需与导入的包名一致。支持别名，例如 `'money "example.com/project/money"'` 搭配 `money.Amount`。
- 映射对所选表中所有同数据库类型的字段生效，沿用 Gen 的可空指针、默认值指针及 unsigned 等后续处理规则。
- 类型语法在加载配置时校验。包是否存在、类型是否实现必要的数据库接口，由目标项目负责；生成代码使用的依赖需要在目标项目中安装。

## 兼容的参数与配置

下表字段都位于 YAML 的 `database` 下，也提供同名命令行参数（例如 `-onlyModel`）。

| 字段 | 默认值 | 用途 |
| --- | --- | --- |
| `dsn` | 空，必填 | 数据库连接字符串 |
| `db` | `mysql` | `mysql` / `postgres` / `sqlite` / `sqlserver` / `clickhouse` |
| `tables` | 所有表 | YAML 使用数组；命令行使用逗号分隔，忽略首尾空白及空项 |
| `onlyModel` | `false` | 仅生成模型 |
| `outPath` | `./dao/query` | 查询代码输出目录 |
| `outFile` | `gen.go` | 查询入口文件名 |
| `modelPkgName` | `model` | 模型包名，或模型输出目录路径；包名默认放在查询目录的同级 |
| `withUnitTest` | `false` | 生成查询单元测试 |
| `unitTestTemplate` | 空 | 自定义查询测试模板路径 |
| `fieldNullable` | `false` | 可空字段使用指针 |
| `fieldCoverable` | `false` | 有默认值的字段使用指针 |
| `fieldWithIndexTag` | `false` | 生成索引标签 |
| `fieldWithTypeTag` | `false` | 生成列类型标签 |
| `fieldWithDefaultTag` | `false` | 生成默认值标签 |
| `fieldSignable` | `false` | 检测无符号整数 |
| `withDefaultQuery` | `false` | 生成默认查询对象 |
| `withoutContext` | `false` | 生成不受 context 约束的查询 API |
| `withQueryInterface` | `false` | 生成导出的查询接口 |
| `withGeneric` | `false` | 生成泛型 API |
| `useAny` | `false` | 使用 `any` 替代 `interface{}` |

## 验证

```bash
CGO_ENABLED=1 go test ./...
go vet ./...
```

集成测试创建临时 SQLite 数据库，从 YAML 加载映射，验证模型及查询输出，
并编译生成的代码（包括带别名的自定义类型包）。SQLite 驱动依赖 CGO，构建和测试需要 C 编译器。

本项目采用 MIT 许可，并保留上游 GORM Gen 的版权声明，详见 [LICENSE](LICENSE)。
