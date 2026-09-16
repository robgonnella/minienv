# minienv

Effortless remote mini environments generated directly from docker compose
config.

You already describe your stack in docker compose config. minienv reads that
config, plus small extension blocks, and deploys it to a remote environment.
Those same extensions can optionally enable public endpoints for the services in
your config, including various authentication mechanisms, so you can collaborate
with others on in-flight work.

## Documentation

For complete documentation, installation instructions, and usage examples for
the current release, visit:

**[https://minienv.rgon.io](https://minienv.rgon.io)**

For documentation for tip of `main`, view mdbook documentation
[SUMMARY.md](./docs/src/SUMMARY.md).

Build it locally with [mdBook](https://rust-lang.github.io/mdBook/):

```sh
just docs-serve
```

## License

Licensed under either of

- Apache License, Version 2.0 ([LICENSE-APACHE](./LICENSE-APACHE) or
  <http://www.apache.org/licenses/LICENSE-2.0>)
- MIT license ([LICENSE-MIT](./LICENSE-MIT) or
  <http://opensource.org/licenses/MIT>)

### Contribution

Unless you explicitly state otherwise, any contribution intentionally submitted
for inclusion in the work by you, as defined in the Apache-2.0 license, shall be
dual licensed as above, without any additional terms or conditions.
