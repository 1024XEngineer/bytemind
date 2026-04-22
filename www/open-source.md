# Open Source

ByteMind is open source software released under the [MIT License](https://github.com/1024XEngineer/bytemind/blob/main/LICENSE).

## Repository

**GitHub:** [https://github.com/1024XEngineer/bytemind](https://github.com/1024XEngineer/bytemind)

The repository contains the full source code, documentation, and release scripts.

## Contributing

We welcome contributions of all kinds — bug fixes, features, documentation improvements, and more.

### Getting Started

1. **Fork** the repository on GitHub.
2. **Clone** your fork locally:

```bash
git clone https://github.com/<your-username>/bytemind.git
cd bytemind
```

3. **Create a branch** for your change:

```bash
git checkout -b feat/your-feature-name
```

4. **Make your changes**, following the guidelines below.
5. **Run the tests:**

```bash
go test ./...
```

6. **Push** your branch and open a **Pull Request** against `main`.

### Guidelines

- Keep changes focused on a single concern. Avoid unrelated refactors in the same PR.
- Prefer existing project patterns and the Go standard library over new abstractions.
- If your change affects the agent's prompt assembly or execution loop, update or add tests in the same PR.
- Write clear commit messages that describe *why*, not just *what*.

### Reporting Issues

Found a bug or have a feature request? [Open an issue](https://github.com/1024XEngineer/bytemind/issues) on GitHub.

Please include:
- ByteMind version (`bytemind --version`)
- Operating system and Go version (if building from source)
- Steps to reproduce the issue
- Expected vs. actual behavior

## License

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
