# Struct 字符串长度规则：性能审查

日期：2026-09-15。

## 结论

- **原有插入路径没有发现明显回退**：28 个未配置长度规则的基准中，B/op 和 allocs/op 全部一致，耗时中位数变化为 −4.0%～+3.3%。
- 缓存命中的元数据读取和扫描准备没有新增分配，耗时变化为 −1.4%～+1.0%。8 字段模型首次建立元数据时，多分配 112 B（1616 → 1728 B，+6.9%），分配次数仍为 16。这是每种模型的缓存建立成本，不是每行插入的成本。
- 原有 `StringLimit.Apply` 的 4 个基准分配一致；耗时增加约 0.2～2.4 ns（+0.3%～+3.6%）。
- **启用规则有额外成本**：100 行、每行一个有效短字符串时，`Structs` 增加约 1.06 μs（+16.5%），`InsertPlan` 增加约 1.72 μs（+41.2%），两者均不新增分配。这里的相对增幅不能称为极小，绝对成本约为每行 11～17 ns；原有插入计划的快速路径不受影响。
- 在本次值模型样本中，实际截断每个字符串增加 16 B、1 次分配；100 行截断时，`Structs`/`InsertPlan` 约为 16～17 μs/批。后续 Build、模板绑定和执行不会重复执行规则。

因此，未启用新功能的既有负载符合“允许极小退化、不能严重退化”的要求；启用规则的批量提取增加了明确可测的处理成本，具体见下表。

## 实现与成本来源

- 标签只在模型元数据建立时解析；没有规则的字段不创建规则对象。
- 字段元数据增加一个规则指针；未配置规则的 `InsertPlan` 保留原来的快速字段读取路径。
- 有规则时复用 `sqltype.StringLimit` 的 UTF-8 检查、计数和截断。新增 `ApplyString` 避免检查过程中把输入、输出反复装入 interface。
- 值未改变时复用反射字段的现有值存储；截断结果才需要新的字符串接口值。未复制字符串内容，截断仍采用原有子串语义。
- 带规则的字符串指针会保存值快照，即使未超限；这类指针字段可能增加快照分配。本次启用规则的性能表使用普通 string 字段，不将其分配结论推广到指针字段。
- 规则在 `Struct`、`Structs`、`InsertPlan.AppendTo` 提取字段时执行。输入模型不被修改；省略/default 判断沿用原始值；规则失败不会到达数据库执行器。

## 测量方法

- 基线：`841134df4b87a3d72b6c9740c6613c763a69302b`。
- 对照：本次实现后的工作区；相同基准源码用于基线和对照。新增的 `BenchmarkInsertStructSingle` 与 `BenchmarkStringLimitApply` 只补充测量，不修改基线实现。
- 环境：Apple M3 Pro，darwin/arm64，macOS 26.6.2，Go 1.27.1。
- 每例 6 个样本；表格使用中位数；关闭 race/coverage 后测量，`-test.cpu 1`。测试编译与基准运行串行进行。
- 插入对比每样本 200 ms，分别运行基线和对照。元数据与 Apply 对比每样本 250 ms，按基线/对照交替运行 6 轮；启用规则样本为 250 ms × 6。
- 最初元数据测量出现明显宿主机噪声，因此最终采用交替复测结果；下方原始记录仅包含最终有效样本。
- 这些是内存中的字段提取/参数准备基准，不包含数据库 I/O，不用于推算完整数据库操作的吞吐；微小耗时变化不作统计显著性结论。

复现时在基线源码副本中加入当前两个新增基准文件，再分别编译：

```sh
# 根包使用 insert_struct_single_benchmark_test.go；sqltype 使用 string_limit_benchmark_test.go。
go test -c -o root.test .
go test -c -o rowbind.test ./internal/rowbind
go test -c -o sqltype.test ./sqltype

./root.test -test.run '^$' -test.bench '^(BenchmarkInsertStructSingle|BenchmarkInsertStructProjection|BenchmarkCompileInsert|BenchmarkInsertPlanInputs|BenchmarkInsertPlanFallbacks|BenchmarkInsertPlanOmitModes)$' -test.benchmem -test.benchtime 200ms -test.count 6 -test.cpu 1
./rowbind.test -test.run '^$' -test.bench '^(BenchmarkMetadata|BenchmarkStructScanPreparation)$' -test.benchmem -test.benchtime 250ms -test.count 1 -test.cpu 1
./sqltype.test -test.run '^$' -test.bench '^BenchmarkStringLimitApply$' -test.benchmem -test.benchtime 250ms -test.count 1 -test.cpu 1
# 上面两个 count=1 命令，在基线与对照之间交替运行 6 轮。

# 仅在实现后的根包运行新增功能成本测试。
./root.test -test.run '^$' -test.bench '^BenchmarkInsertStringLimitCosts$' -test.benchmem -test.benchtime 250ms -test.count 6 -test.cpu 1
```

## 完整前后对比

正值表示耗时增加。B/op 为累计分配字节数，不代表常驻堆大小。

| 基准（省略 Benchmark 前缀） | 之前 ns/op | 之后 ns/op | 耗时变化 | B/op 前→后 | allocs/op 前→后 |
| --- | ---: | ---: | ---: | ---: | ---: |
| InsertStructProjection/same/1 | 178.70 | 179.45 | +0.4% | 424→424 | 4→4 |
| InsertStructProjection/alternating/1 | 166.35 | 171.50 | +3.1% | 416→416 | 3→3 |
| InsertStructProjection/same/100 | 3379.00 | 3490.00 | +3.3% | 2992→2992 | 103→103 |
| InsertStructProjection/alternating/100 | 6416.50 | 6564.00 | +2.3% | 2192→2192 | 3→3 |
| CompileInsert/cols3 | 194.25 | 194.15 | -0.1% | 208→208 | 3→3 |
| CompileInsert/cols12 | 667.75 | 675.05 | +1.1% | 1016→1016 | 6→6 |
| InsertPlanInputs/values/structs | 6013.00 | 6135.50 | +2.0% | 8520→8520 | 104→104 |
| InsertPlanInputs/values/plan | 3998.50 | 3934.00 | -1.6% | 8496→8496 | 103→103 |
| InsertPlanInputs/pointers/structs | 8851.00 | 8924.50 | +0.8% | 8520→8520 | 304→304 |
| InsertPlanInputs/pointers/plan | 6395.50 | 6510.50 | +1.8% | 8496→8496 | 303→303 |
| InsertPlanInputs/pointer_valuer/structs | 6822.00 | 6908.00 | +1.3% | 6296→6296 | 204→204 |
| InsertPlanInputs/pointer_valuer/plan | 5035.50 | 5055.50 | +0.4% | 6272→6272 | 203→203 |
| InsertPlanFallbacks/rows1/pointer_parent/structs | 229.15 | 235.70 | +2.9% | 496→496 | 6→6 |
| InsertPlanFallbacks/rows1/pointer_parent/plan | 180.00 | 185.35 | +3.0% | 472→472 | 5→5 |
| InsertPlanFallbacks/rows1/omit_zero/structs | 215.75 | 211.00 | -2.2% | 488→488 | 5→5 |
| InsertPlanFallbacks/rows1/omit_zero/plan | 165.15 | 158.50 | -4.0% | 464→464 | 4→4 |
| InsertPlanFallbacks/rows1000/pointer_parent/structs | 59887.50 | 59615.00 | -0.5% | 53208→53208 | 1504→1504 |
| InsertPlanFallbacks/rows1000/pointer_parent/plan | 44381.50 | 45016.50 | +1.4% | 53184→53184 | 1503→1503 |
| InsertPlanFallbacks/rows1000/omit_zero/structs | 57665.00 | 58776.50 | +1.9% | 65208→65208 | 1504→1504 |
| InsertPlanFallbacks/rows1000/omit_zero/plan | 42368.50 | 42428.50 | +0.1% | 65184→65184 | 1503→1503 |
| InsertPlanOmitModes/rows1/implicit | 182.65 | 180.75 | -1.0% | 496→496 | 5→5 |
| InsertPlanOmitModes/rows1/explicit | 159.70 | 158.25 | -0.9% | 464→464 | 4→4 |
| InsertPlanOmitModes/rows1000/implicit | 50916.00 | 50790.50 | -0.2% | 81184→81184 | 2003→2003 |
| InsertPlanOmitModes/rows1000/explicit | 24973.50 | 24399.00 | -2.3% | 49184→49184 | 1003→1003 |
| InsertStructSingle/value/implicit | 467.90 | 464.85 | -0.7% | 880→880 | 8→8 |
| InsertStructSingle/value/explicit | 438.75 | 424.85 | -3.2% | 816→816 | 6→6 |
| InsertStructSingle/pointer/implicit | 512.90 | 512.10 | -0.2% | 912→912 | 11→11 |
| InsertStructSingle/pointer/explicit | 495.45 | 485.90 | -1.9% | 848→848 | 9→9 |
| StructScanPreparation/reused | 122.95 | 123.50 | +0.4% | 0→0 | 0→0 |
| StructScanPreparation/fresh | 307.20 | 304.35 | -0.9% | 704→704 | 3→3 |
| StructScanPreparation/alternating | 91.81 | 92.73 | +1.0% | 0→0 | 0→0 |
| StructScanPreparation/cold | 5764.50 | 5721.00 | -0.8% | 2616→2728 | 23→23 |
| Metadata/warm | 11.81 | 11.64 | -1.4% | 0→0 | 0→0 |
| Metadata/cold | 5180.00 | 5189.50 | +0.2% | 1616→1728 | 16→16 |
| StringLimitApply/valid | 24.69 | 25.57 | +3.6% | 16→16 | 1→1 |
| StringLimitApply/truncate | 108.85 | 111.25 | +2.2% | 16→16 | 1→1 |
| StringLimitApply/bytes | 86.27 | 87.55 | +1.5% | 16→16 | 1→1 |
| StringLimitApply/reject | 65.33 | 65.56 | +0.3% | 24→24 | 1→1 |

## 启用规则后的成本

同为三个字段的值模型，`Name` 配置 `maxlen=64,overflow=truncate`，默认按 rune 计数。

- `plain`：无规则，字符串为 `alice`。
- `valid`：有规则，字符串为 `alice`。
- `truncate`：有规则，将 128 个 ASCII 字符截断为 64 个。
- `struct`：逐行调用 `Struct`；`structs`：一次批量调用；`plan`：预先编译计划，计时期间调用 `AppendTo`。

下表是实现后新增功能的成本比较，不是原有规则失效情况下的等价性能回归比较。

| 行数/数据/入口 | ns/op（整批） | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| rows1/plain/struct | 484.80 | 912 | 9 |
| rows1/plain/structs | 252.80 | 536 | 5 |
| rows1/plain/plan | 180.55 | 512 | 4 |
| rows1/valid/struct | 487.40 | 912 | 9 |
| rows1/valid/structs | 269.80 | 536 | 5 |
| rows1/valid/plan | 201.25 | 512 | 4 |
| rows1/truncate/struct | 578.45 | 928 | 10 |
| rows1/truncate/structs | 370.50 | 552 | 6 |
| rows1/truncate/plan | 301.65 | 528 | 5 |
| rows100/plain/struct | 35514.50 | 44328 | 416 |
| rows100/plain/structs | 6435.50 | 8520 | 104 |
| rows100/plain/plan | 4175.00 | 8496 | 103 |
| rows100/valid/struct | 35192.50 | 44328 | 416 |
| rows100/valid/structs | 7499.00 | 8520 | 104 |
| rows100/valid/plan | 5896.00 | 8496 | 103 |
| rows100/truncate/struct | 42651.00 | 45928 | 516 |
| rows100/truncate/structs | 16808.50 | 10120 | 204 |
| rows100/truncate/plan | 16118.00 | 10096 | 203 |

## 验证与原始记录

`go test -race -cover ./...` 全部通过。新增测试覆盖 Unicode/字节边界、NULL/嵌套指针、显式列、省略/default、模型快照、错误传递、执行器不被调用、计划回滚、计划并发复用，以及查询和扫描不转换字段。

- [基线基准记录](benchmarks/struct-string-limits/before.txt)
- [实现后基准记录](benchmarks/struct-string-limits/after.txt)
- [启用规则的成本记录](benchmarks/struct-string-limits/costs.txt)
- [Race 与覆盖率结果](benchmarks/struct-string-limits/race-cover.txt)
