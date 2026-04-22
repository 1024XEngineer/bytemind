# 开源参与

ByteMind 是在 [MIT 许可证](https://github.com/1024XEngineer/bytemind/blob/main/LICENSE) 下发布的开源软件。

## 仓库地址

**GitHub：** [https://github.com/1024XEngineer/bytemind](https://github.com/1024XEngineer/bytemind)

仓库包含完整的源代码、文档和发布脚本。

## 参与贡献

我们欢迎各类贡献 — Bug 修复、新功能、文档改进等。

### 快速开始

1. 在 GitHub 上 **Fork** 仓库。
2. **克隆**你的 Fork 到本地：

```bash
git clone https://github.com/<你的用户名>/bytemind.git
cd bytemind
```

3. 为你的改动**创建分支**：

```bash
git checkout -b feat/your-feature-name
```

4. **进行修改**，遵循以下开发规范。
5. **运行测试**：

```bash
go test ./...
```

6. **推送**分支并向 `main` 发起 **Pull Request**。

### 开发规范

- 每个 PR 聚焦于单一关注点，避免混入无关重构。
- 优先使用项目现有模式和 Go 标准库，而非引入新的抽象层。
- 如果改动影响 Agent 的 Prompt 组装或执行循环，请在同一 PR 中更新或新增测试。
- 写清晰的提交信息，说明*为什么*做出改变，而非仅描述*做了什么*。

### 提交 Issue

发现 Bug 或有功能建议？在 GitHub 上[提交 Issue](https://github.com/1024XEngineer/bytemind/issues)。

请包含以下信息：
- ByteMind 版本（`bytemind --version`）
- 操作系统以及 Go 版本（如从源码构建）
- 复现步骤
- 预期行为与实际行为

## 开源许可证

```
MIT License

Copyright (c) 2024-present ByteMind Contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
