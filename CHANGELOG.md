# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-01-08

### Added
- Multi-stage comparison pipeline (Byte-wise -> Structural -> Visual).
- Rendering powered by PDFium via WebAssembly (wazero).
- Perceptual hashing (dHash and pHash) for visual similarity.
- Strategic sampling for large documents ($\sqrt{n} + 2$ pages).
- Concurrent page processing and hashing.
- Color histogram pre-check for fast rejection.
- Text character count and graphical object count heuristics.
- CLI tool with `compare` and `quick` commands.
- JSON output support for machine-readable results.
- Comprehensive benchmark suite.
