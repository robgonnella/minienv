# [0.3.0](https://github.com/robgonnella/minienv/compare/v0.2.1...v0.3.0) - 2026-09-18

### 🚀 Features

- adds ability to configure removal of k8s namespace on destroy [_(2c1981c)_](https://github.com/robgonnella/minienv/commit/2c1981ca5c72ed6f8e1c62e87c89f460cc745364)

# [0.2.1](https://github.com/robgonnella/minienv/compare/v0.2.0...v0.2.1) - 2026-09-17

### 🐛 Bug Fixes

- fixes two minor issues [_(bad30b9)_](https://github.com/robgonnella/minienv/commit/bad30b931f704a40487ebe32dc269bfc6c866aa2)

### 📚 Documentation

- updates documentation [_(5a8306f)_](https://github.com/robgonnella/minienv/commit/5a8306f06372245027ebe52610d70cd6cabd7146)

# [0.2.0](https://github.com/robgonnella/minienv/compare/v0.1.2...v0.2.0) - 2026-09-17

### 🚀 Features

- _(k8s)_ adds support for generating config maps from file [_(2510d29)_](https://github.com/robgonnella/minienv/commit/2510d29589dcfffdc784ffc2c6204cad225313d5)

### 🐛 Bug Fixes

- _(k8s)_ fixes issue with k8s env [_(b26c7e0)_](https://github.com/robgonnella/minienv/commit/b26c7e01482dbff2c9b67891bc3fb69fe7a5d5df)

# [0.1.2](https://github.com/robgonnella/minienv/compare/v0.1.1...v0.1.2) - 2026-09-16

### ⏩ CI/CD

- minor update to publish workflow [_(686e0d8)_](https://github.com/robgonnella/minienv/commit/686e0d8b06cf7a97ec3377bea5ab3ce933567be8)

# [0.1.1](https://github.com/robgonnella/minienv/compare/v0.1.0...v0.1.1) - 2026-09-16

### ⏩ CI/CD

- fixes issue with publish workflow [_(ac014ff)_](https://github.com/robgonnella/minienv/commit/ac014ffc9860988214327a64a64861560a8d3658)

# [0.1.0](https://github.com/robgonnella/minienv/releases/tag/v0.1.0) - 2026-09-16

### 🚀 Features

- _(docker)_ enables users to copy and mount additional files [_(58b0858)_](https://github.com/robgonnella/minienv/commit/58b0858194614d34d8914b5e59c0dbdb7bdfbe16)

- adds ability to specify additional k8s manifests [_(458e4bc)_](https://github.com/robgonnella/minienv/commit/458e4bc381065d5afd35342295628397d574178b)

- adds docker deployer with ssh transport [_(df8f6a3)_](https://github.com/robgonnella/minienv/commit/df8f6a36bc284eb60c80e201e0948b89936763e3)

- adds support for k8s jobs [_(06400d5)_](https://github.com/robgonnella/minienv/commit/06400d58357570e0ca9588f48c22517043529f4f)

- implements dependency aware deployments for helm [_(40006ea)_](https://github.com/robgonnella/minienv/commit/40006ea40b9ecc50334eb1ff378b605ce3c01584)

- adds support for building and pushing images [_(f5f69ed)_](https://github.com/robgonnella/minienv/commit/f5f69ed2d470c6c57c3ca5780e7f8b92fc64e620)

- adds support for publishing via ngrok [_(9d83c5e)_](https://github.com/robgonnella/minienv/commit/9d83c5e803d562907ccd29c8bc17751a4b28dbb6)

- initial prototype [_(258f5d5)_](https://github.com/robgonnella/minienv/commit/258f5d583b65d25b34121753fda19b8ace3336bb)

- adds script for generating json schema [_(575e2f7)_](https://github.com/robgonnella/minienv/commit/575e2f78941fec71a370f715e3e9dfc542e0878a)

### 🐛 Bug Fixes

- uses an explicit "transport" field for docker extension [_(675fa30)_](https://github.com/robgonnella/minienv/commit/675fa3095556c4e332482fe3bc45607858b24c0f)

- fixes filepaths and docs [_(9cff013)_](https://github.com/robgonnella/minienv/commit/9cff01327ec1bec0bf05c0faea5aaf305aa27d8f)

- simplifies k8s port mappings [_(033fc38)_](https://github.com/robgonnella/minienv/commit/033fc38d376fdfda403954dad92f18630b4dd40b)

- uses cached short-sha where possible in git client [_(6b475e3)_](https://github.com/robgonnella/minienv/commit/6b475e3364120bc8bd8511abf646a9f3a81a79a3)

- cleanup [_(b40114f)_](https://github.com/robgonnella/minienv/commit/b40114f62867a5ba7d60b7ef9bc4a487f83c7bbd)

- fixes issue with ngrok design [_(a406328)_](https://github.com/robgonnella/minienv/commit/a406328bcececf416bb138276b0393812eab362d)

### 🚜 Refactor

- tightens linting rules and updates code to conform [_(1fb7f85)_](https://github.com/robgonnella/minienv/commit/1fb7f85773ebaefef10574e45597bffcd5be6a5f)

- moves helm files to dedicated helm package within deployer [_(0fb3aca)_](https://github.com/robgonnella/minienv/commit/0fb3aca0648db45be370ac4d283aa9c444d6163e)

- refactors internal core [_(fafe1dc)_](https://github.com/robgonnella/minienv/commit/fafe1dc21a01e4ecf3e6a162f30f4b43c2e2f13f)

- simplifies config handling [_(6225f64)_](https://github.com/robgonnella/minienv/commit/6225f64bdd94cebdd26f744cd1d87ebd554262d3)

- uses go text/template to generate buildx hcl string [_(17f4128)_](https://github.com/robgonnella/minienv/commit/17f41282e808fb6cc0761271cc5f3467c21b9aed)

- creates separate image package for image handling [_(5518848)_](https://github.com/robgonnella/minienv/commit/551884819bf9108583b28c863455fbe468c0831a)

### 📚 Documentation

- adds documentation in mdbook format [_(09721d1)_](https://github.com/robgonnella/minienv/commit/09721d196844044ac7527017a63522ab8f4b8a60)

### 🧪 Testing

- adds unit tests [_(df0db58)_](https://github.com/robgonnella/minienv/commit/df0db58477e7779dad65287ed7ccdd8f3d866da8)

### 🧹 Chore

- add licenses and update README.md [_(77531d7)_](https://github.com/robgonnella/minienv/commit/77531d7a1837f80514abda55794f5ed6a5c1d676)

- splits up helm tests over multiple files [_(5737bd2)_](https://github.com/robgonnella/minienv/commit/5737bd2e4f7473cb1a0d9e291ff6a59b4f2b7d3c)

- adds mocking and unit testing setup [_(b19ccaf)_](https://github.com/robgonnella/minienv/commit/b19ccafb9d7e1e7f69642b288a215d7fdf36a4a2)

- initial commit [_(b8fe389)_](https://github.com/robgonnella/minienv/commit/b8fe3894bc3c7f8ad0d1ffb2152acf49a295e43f)

- adds golangci-lint and justfile lint recipe [_(affa928)_](https://github.com/robgonnella/minienv/commit/affa92844207954fb1f9142d3b1b74f16ff46f29)

- adds initial test suite setup [_(481fd03)_](https://github.com/robgonnella/minienv/commit/481fd03c06c93654d1a8b3fb95825bd51c4f3c47)

### ⏩ CI/CD

- adds publish workflow [_(1a25e5e)_](https://github.com/robgonnella/minienv/commit/1a25e5ecd5be7c8d6663a89b4436cae274c58992)

- adds docs workflow [_(b07de3f)_](https://github.com/robgonnella/minienv/commit/b07de3f6a40343e5dfae82953ec33869904f77eb)

- adds release workflow [_(e188ed9)_](https://github.com/robgonnella/minienv/commit/e188ed9b57d5538f24bc0a8e28da9c0269ba78c3)

- adds ci pipeline [_(bf14049)_](https://github.com/robgonnella/minienv/commit/bf14049c42a5e79708568151e8c2205d3e21dce3)
