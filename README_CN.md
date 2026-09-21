# procfind-kit

[![CI](https://github.com/soulteary/procfind-kit/actions/workflows/ci.yml/badge.svg)](https://github.com/soulteary/procfind-kit/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/soulteary/procfind-kit.svg)](https://pkg.go.dev/github.com/soulteary/procfind-kit)
[![Go Report Card](.github/goreportcard.svg)](.github/goreportcard-report.md)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

为「不写 pid 文件」的程序，按目录找出正在运行的进程。零依赖。

**文档:** [English](README.md) · 中文

## 要解决的问题

一个守护程序只需要回答一个问题：*我的 worker 还活着吗？* 通常的答案是 pid 文件 —— 但相当多的程序根本不写。

本包抽取自 GitHub Actions runner 的场景。它的三个启动脚本没有任何一处记录 pid，那个值只存在于 shell 变量里；看起来像 pid 文件的 `.path`，装的其实是 `PATH` 字符串。于是照着找 pid 文件的代码**每一次、对每一个 runner** 都找不到，进而判定进程已死。

守护程序按这个结论行事，就会再拉起一份。每次巡检都拉一份。

本包改为去问进程表：一个进程的 `argv` 指向某目录下的文件，它就属于那个目录。

## 安装

```bash
go get github.com/soulteary/procfind-kit
```

## 用法

先描述「你的进程长什么样」，然后查询：

```go
import procfind "github.com/soulteary/procfind-kit"

spec := procfind.Spec{
    Scripts:     []string{"run.sh", "run-helper.sh"},
    Executables: []string{"bin/Runner.Listener"},
}

if procfind.Running("/srv/runners/build-01", spec) {
    // ...
}

pids := procfind.Find("/srv/runners/build-01", spec)
```

一次列出多个目录时，进程表**只扫一遍**，而不是每个目录扫一遍：

```go
byDir := procfind.FindMany([]string{"/srv/a", "/srv/b", "/srv/c"}, spec)
```

## 为什么 `Scripts` 与 `Executables` 要分成两个列表

两个原因，都是踩出来的。

**它们在进程表里的形态不同。** 通过 shebang 启动的脚本，进程其实是*解释器*在跑脚本，`argv` 形如 `["/bin/bash", "/srv/app/run.sh"]` —— 路径落在 `argv[1]`。以绝对路径启动的二进制，路径就是 `argv[0]`。不经 shebang 直接 exec 的脚本，同样落在 `argv[0]`。三种都能认出来。

**结果顺序是「脚本在前」。** 监护脚本通常在循环里重启它的 worker。要把整套停掉，必须先给脚本发信号；先杀 worker，循环会立刻再拉起一个。`Find` 把脚本排在可执行文件之前，于是

```go
for _, pid := range procfind.Find(dir, spec) {
    syscall.Kill(pid, syscall.SIGTERM)
}
```

不需要调用方知道这条规则，就是对的。

## 刻意不做的事

**只看 `argv[0]` 与 `argv[1]`，且只有 `argv[0]` 是 shell 时才认 `argv[1]`。** 扫整条命令行会把 `grep /srv/app/run.sh`、`tail -f /srv/app/run.sh` 也算进去 —— 而运维正在排查「守护程序为什么反复重启」时，极有可能就在跑这类命令。把它们算作「服务还在」，等于把故障藏起来。

**读 `cmdline`，不读 `exe` 符号链接。** `/proc/<pid>/cmdline` 对同主机任何用户可读；而对不属于自己的进程 `readlink /proc/<pid>/exe` 会 `EACCES`，除非持有 `CAP_SYS_PTRACE`。一个以普通服务账号运行的守护程序，不该为了回答「我的 worker 活着吗」而需要那种权限。

**仅限 Linux。** 没有 procfs 就无从回答，此时一律报告「找不到」—— 这是诚实的答案，而不是往任一方向猜。

## 测试

`Scanner.Root` 可以把扫描指向另一个 procfs 挂载点，于是你自己的测试可以造一个 fixture 目录，而不必真的起进程：

```go
s := procfind.Scanner{Root: "testdata/proc"}
pids := s.Find("/srv/app", spec)
```

包级的 `Find` / `FindMany` / `Running` 就是 `Root` 取默认值 `/proc` 的 `Scanner{}`。

## API

| 函数 | 说明 |
|---|---|
| `Find(dir string, spec Spec) []int` | 属于 `dir` 的 pid，脚本在前 |
| `FindMany(dirs []string, spec Spec) map[string][]int` | 多目录版本，进程表只扫一遍 |
| `Running(dir string, spec Spec) bool` | `dir` 下是否有进程存活 |
| `Scanner{Root}` | 同样三个方法，指定 procfs 挂载点 |
| `Spec{Scripts, Executables}` | 相对目录的路径，用于识别进程 |

`FindMany` 按传入的原字符串索引结果 —— 同一目录的两种写法各有一个条目，内容相同。

## 要求

- **Go 1.27+**（`go.mod` 中声明 `go 1.27.0`）
- **零依赖。** 连测试在内，全部只用标准库。
- **需要有 procfs 可读。** 本包在任何平台都能编译和运行 —— 在 Windows 上、以及
  任何没有挂载 `/proc` 的地方，每次查询都会如实报告「没找到」，而不是去猜。
  `/proc` 以 `hidepid=1` 或 `hidepid=2` 挂载时，对属于其他用户的进程也是同样
  的结果。

## 测试覆盖率

```bash
go test ./... -v

# 带覆盖率 —— CI 实际执行的命令
go test -race -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -html=coverage.out -o coverage.html
go tool cover -func=coverage.out
```

语句覆盖率为 **96.6%**，且没有一个测试需要真实的进程表 —— `Scanner.Root` 指向
一个 fixture 目录即可。CI 每次运行都会把可浏览的 HTML 报告作为构建产物上传；
不接入任何覆盖率服务。

`example_test.go` 里的可运行示例是测试套件的一部分。它们是*外部*测试包
（`package procfind_test`），只能编译到导出的 API —— 这能逼着这套 API 对包外
调用者保持可用 —— 而且 `go test` 会校验它们打印的输出，因此示例不会与文档
所述发生偏移。

## 变更日志

见 [CHANGELOG.md](CHANGELOG.md)。

## 安全

匹配上了并不等于身份得到了证明：argv 由启动进程的人设定，因此任何本地用户都能
让 `Running` 返回 true。这一点、调用方发信号时要面对的 pid 复用窗口，以及本包
为什么读 `cmdline` 而不是去跟 `/proc/<pid>/exe` 链接，都写在
[SECURITY.md](SECURITY.md) 里 —— 一并还有如何上报安全问题。请不要为安全问题开
公开 issue。

## 贡献

1. Fork 本仓库
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add some amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 提交 Pull Request

## 许可证

Apache 2.0，见 [LICENSE](LICENSE)。
