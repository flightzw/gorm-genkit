# gorm-genkit

参考 [GORM gentool](https://github.com/go-gorm/gen/blob/master/tools/gentool/gentool.go)
实现的数据库代码生成工具，支持 MySQL、PostgreSQL、SQLite、SQL Server 和 ClickHouse。
可以生成模型、查询代码及查询测试，并通过 YAML 配置数据库字段类型到 Go 类型的映射。

## 安装

需要 Go 1.26.8 或更新版本。安装工具本身不强制启用 CGO。
仅使用 MySQL、PostgreSQL、SQL Server 或 ClickHouse 时，可以用 `CGO_ENABLED=0` 安装。

在本仓库根目录安装：

```bash
go install ./cmd/genkit
```

仓库代码推送到可访问的 GitHub 仓库后，也可从任意目录安装：

```bash
go install github.com/flightzw/gorm-genkit/cmd/genkit@latest
```

如果需要生成 **SQLite** 数据库的代码，必须在构建或安装工具时启用 CGO，
并在构建机器上安装 C 编译器（如 GCC 或 Clang）：

```bash
CGO_ENABLED=1 go install ./cmd/genkit
# 从远程安装时同样在 go install 命令前设置 CGO_ENABLED=1
```

`CGO_ENABLED=0` 也能安装成功，但生成的可执行文件不支持连接 SQLite。
运行已安装的工具时再设置 `CGO_ENABLED=1` 不会改变其能力，需要重新构建或安装。
可用 `go env CGO_ENABLED` 查看当前构建环境的默认设置。

可执行文件安装到 `go env GOBIN` 指定的目录；未设置 `GOBIN` 时，默认安装到
`$(go env GOPATH)/bin`。将对应目录加入 `PATH`。例如，使用默认安装目录时：

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
genkit -h
```

可将上述 `export` 写入 shell 配置文件，使其在后续终端会话中生效。安装完成后，可在目标 Go 项目中直接使用 `genkit`。

## 安装后使用

进入需要生成代码的 Go 项目根目录，参考 [gen.yml](gen.yml) 创建项目自己的配置文件，
填写数据库连接信息、所选表和输出目录。例如：

```yaml
version: "1"
database:
  db: mysql
  dsn: "root:password@tcp(127.0.0.1:3306)/example?charset=utf8mb4&parseTime=True&loc=Local"
  tables: [users, orders]
  outPath: ./dao/query
  modelPkgName: model
  withDefaultQuery: true
```

保存为 `gen.yml` 后执行：

```bash
genkit -c gen.yml
```

上述配置将模型生成到 `./dao/model`，将查询代码生成到 `./dao/query`，查询入口文件默认为 `gen.go`。
`tables` 留空或设为 `[]` 时生成所有表。生成代码及自定义类型所需的依赖，应安装在目标 Go 项目中。

也可以直接使用命令行参数。以下示例读取已有的 SQLite 数据库：

```bash
# 生成指定表的模型和查询代码
genkit -db sqlite -dsn ./example.db -tables users,orders -outPath ./dao/query

# 只生成模型
genkit -db sqlite -dsn ./example.db -tables users,orders -onlyModel -outPath ./dao/query

# 查看全部参数
genkit -h
```

指定 `-c` 时，配置文件优先，其他生成参数不参与合并；`-envFile` 仍生效。
连接字段为空时先从环境变量补充，其他未配置字段使用默认值。
所有相对路径（包括 SQLite DSN、输出目录和模板路径）均相对于运行命令时的工作目录。
配置文件使用上游的 `version` / `database` 结构；未知字段、重复键和无效类型配置会报错。

如果需要从源码运行，也可在本仓库根目录执行：

```bash
go run ./cmd/genkit -c gen.yml
```

从源码运行 SQLite 配置时，使用 `CGO_ENABLED=1 go run ./cmd/genkit -c gen.yml`。

## 使用 .env 配置数据库连接

工具通过 [godotenv](https://github.com/joho/godotenv) 默认加载工作目录下的 `.env`。
可参考 [.env.example](.env.example) 创建文件：

```dotenv
GENKIT_DB=mysql
GENKIT_DSN='root:password@tcp(127.0.0.1:3306)/example?charset=utf8mb4&parseTime=True&loc=Local'
```

不指定连接参数时，直接使用上述环境变量：

```bash
genkit -tables users,orders -outPath ./dao/query
```

也可以在 YAML 的 `db`、`dsn` 字段中引用任意环境变量：

```yaml
version: "1"
database:
  db: "${GENKIT_DB}"
  dsn: "${GENKIT_DSN}"
  tables: [users, orders]
  outPath: ./dao/query
  modelPkgName: model
```

```bash
genkit -c gen.yml
genkit -envFile ./config/.env.local -c gen.yml
genkit -envFile="" -c gen.yml # 仅使用系统环境变量，不加载文件
```

- 优先使用选定配置来源（`-c` 指定的 YAML，或命令行）中的连接字段；空字段分别回退到 `GENKIT_DB`、`GENKIT_DSN`，数据库类型最终默认为 `mysql`，DSN 仍为空时报错。
- 系统中已有的环境变量优先于 `.env` 中的值，不会被覆盖。
- 默认 `.env` 不存在时继续运行；显式指定的文件不存在、文件无法读取或格式错误时，返回错误。
- `-envFile` 相对路径基于工作目录，不基于 YAML 文件目录。安装工具不会自动创建或安装 `.env` 文件。
- 仅对选定配置的 `db`、`dsn` 字段展开 `${变量名}`；未定义的变量会报错，不展开其他 YAML 字段或 `$NAME`。先解析 YAML 再展开连接字段，变量值中的冒号、引号等不会改变 YAML 结构。
- `.env` 内的变量引用遵循 godotenv 语法；密码含字面量 `$` 时，建议将 `.env` 值用单引号包围。本仓库已忽略 `.env` 和 `.env.*`，保留 `.env.example` 作为模板。

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

## 软删除字段类型

未显式配置 `softDelete` 时，默认对时间类型的 `deleted_at` 列使用 `gorm.DeletedAt`，
优先于 `dataTypeMap`；非时间列继续使用 `dataTypeMap`，没有匹配项时使用 Gen 默认映射。
该默认行为同时适用于 YAML 和纯命令行模式。
通过仅限 YAML 的 `softDelete` 配置，可以覆盖软删除列名及 Go 类型：

```yaml
database:
  db: "${GENKIT_DB}"
  dsn: "${GENKIT_DSN}"
  softDelete:
    column: deleted_at
    type: gorm.DeletedAt
  dataTypeMap:
    datetime: time.Time
```

此例中，时间类型的 `deleted_at` 最终生成 `gorm.DeletedAt`，其他 `datetime` 列仍按 `dataTypeMap` 生成。

- `column` 是数据库列名，省略或设为空字符串时默认为 `deleted_at`；按列名精确匹配，对所有生成的表生效。表中不存在该列时不添加字段。
- `type` 必填，支持与 `dataTypeMap` 相同的 Go 类型表达式；自定义包通过 `importPkgPaths` 导入。例如将 `column` 改为 `archived_at`、`type` 改为 `custom.DeletedAt`，即可覆盖其他列名和类型。
- `gorm.DeletedAt`（包括指针形式）会根据数据库原始列类型检查兼容性，不根据 `dataTypeMap` 映射后的 Go 类型判断。支持各数据库的日期、日期时间及时间戳类型，例如 MySQL 的 `date/datetime/timestamp`、PostgreSQL 的 `date/timestamp/timestamptz`、SQL Server 的 `date/datetime/datetime2/smalldatetime/datetimeoffset`、SQLite 的 `date/datetime/timestamp`、ClickHouse 的 `Date/Date32/DateTime/DateTime64`（含 Nullable 等包装）。
- 整数、文本、布尔、未知类型及仅含时分秒的 `TIME` 列不应用 `gorm.DeletedAt`，而是保留普通类型映射及指针规则。SQL Server 的 `timestamp/rowversion` 是二进制类型，也不视为时间列。例如 `deleted_at` 为 `bigint` 且配置 `bigint: uint64` 时，生成 `uint64`（可空时按配置生成指针）。
- 显式配置的其他类型（如 `soft_delete.DeletedAt` 或自定义类型）仍按配置生成，以支持整数或标记位软删除；工具不推断任意自定义类型的数据库兼容性。MySQL 时间列仍需正确设置 DSN 的 `parseTime=True`，列类型检查不代替驱动连接配置。
- 配置的类型作为最终字段类型，优先于 `dataTypeMap`、Gen 默认软删除类型转换及 `fieldNullable` / `fieldCoverable` / `fieldSignable`。需要指针时显式填写 `*gorm.DeletedAt`，工具不会再自动追加指针。
- 省略 `softDelete` 或设为 `null` 时，使用默认配置 `{column: deleted_at, type: gorm.DeletedAt}`。显式配置时 `type` 必填，不能使用空对象 `{}`；配置错误或 Go 类型语法错误会在加载 YAML 时返回错误。
- 此配置指定生成字段的类型。实际软删除行为由该类型及 GORM 的支持决定；`gorm.DeletedAt` 可直接使用，自定义类型需自行实现对应行为。

## 兼容的参数与配置

下表字段都位于 YAML 的 `database` 下，也提供同名命令行参数（例如 `-onlyModel`）。

| 字段 | 默认值 | 用途 |
| --- | --- | --- |
| `dsn` | `GENKIT_DSN`，最终不能为空 | 数据库连接字符串，支持 `${变量名}` |
| `db` | `GENKIT_DB` 或 `mysql` | `mysql` / `postgres` / `sqlite` / `sqlserver` / `clickhouse`，支持 `${变量名}` |
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
并编译生成的代码（包括带别名的自定义类型包）。这部分集成测试需要启用 CGO 并安装 C 编译器。
`CGO_ENABLED=0 go test ./...` 可以运行其他测试，但会跳过 SQLite 集成测试。

本项目采用 MIT 许可，并保留上游 GORM Gen 的版权声明，详见 [LICENSE](LICENSE)。
