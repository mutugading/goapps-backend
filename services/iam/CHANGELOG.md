# Changelog

## [0.10.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.9.0...iam-service/v0.10.0) (2026-04-15)


### Features

* implement employee level management module and fix admin email verification migration ([45cff67](https://github.com/mutugading/goapps-backend/commit/45cff67eb0cbc83cbc910b912c2437ae62cfd6eb))
* implement employee level management module and fix admin email verification migration ([92428c6](https://github.com/mutugading/goapps-backend/commit/92428c61869c42b1333f3aa9ac440f4891d63f4b))


### Bug Fixes

* improve code safety and maintainability by implementing safe integer conversions, extracting entity validation methods, and enforcing linting standards. ([bf248da](https://github.com/mutugading/goapps-backend/commit/bf248dad4e2f6c56e130ed8fba454aaaee0092ec))

## [0.9.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.8.2...iam-service/v0.9.0) (2026-04-14)


### Features

* Add build and copy steps for the `iam-seed` binary. ([b55b3f8](https://github.com/mutugading/goapps-backend/commit/b55b3f89c6b4a66511a944ae7ad1ec9933793110))
* Add golang-migrate and migration files to the IAM service Dockerfile for database migrations. ([1de4a14](https://github.com/mutugading/goapps-backend/commit/1de4a147c48dc90e56e40c0fd0e1ee4f00834e05))
* Add MinIO storage integration and user profile picture upload functionality. ([2754734](https://github.com/mutugading/goapps-backend/commit/27547346f3a6096fe3d4672a8fa7508f9835f2c3))
* Add MinIO storage integration and user profile picture upload functionality. ([8c8d7c0](https://github.com/mutugading/goapps-backend/commit/8c8d7c042e497002579b078fb57c6d2fb3ef9ff4))
* add UOM category management with CRUD, import/export, and seed migration ([29e4c43](https://github.com/mutugading/goapps-backend/commit/29e4c432fdaf2ac13f81dd548488ed6d5596c75f))
* add UOM category management with CRUD, import/export, and seed migration ([f4cbaaa](https://github.com/mutugading/goapps-backend/commit/f4cbaaaed866fec031f8a0af05bdb98970d99254))
* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([c82c258](https://github.com/mutugading/goapps-backend/commit/c82c258bd39279f31789b9635367b2e4cb34d19b))
* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([a1ad398](https://github.com/mutugading/goapps-backend/commit/a1ad3980e6594d80e4154b0ecf4dfcb8f42f09c1))
* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([5eb8515](https://github.com/mutugading/goapps-backend/commit/5eb8515c646e89a934bbd5694c4b9833c6accad8))
* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([c0df3b4](https://github.com/mutugading/goapps-backend/commit/c0df3b47eb36a97bf1eec792ac114ffa4d9571a8))
* implement finance parameter management module with CRUD, import/export, and UI components ([9505fd1](https://github.com/mutugading/goapps-backend/commit/9505fd10daa3aedf88a101e1aea7d0dfe572bcd3))
* implement formula management service with CRUD operations, gRPC definitions, and database schema ([ee9831c](https://github.com/mutugading/goapps-backend/commit/ee9831c156013d0b81b3634645934392dffef324))
* implement formula management service with CRUD operations, gRPC definitions, and database schema ([1faa10c](https://github.com/mutugading/goapps-backend/commit/1faa10ce43da4772131957ef9f53ce95987e5038))
* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([55c70c8](https://github.com/mutugading/goapps-backend/commit/55c70c82a410ec6dbc328a1f0c1bec7609c6b84d))
* Implement session idle timeout, add comprehensive E2E tests, and include a backend run guide. ([5610f2c](https://github.com/mutugading/goapps-backend/commit/5610f2cc28d10f2c2a52db56fb24f539c98962a5))
* Implement the initial Identity and Access Management (IAM) serv… ([f950837](https://github.com/mutugading/goapps-backend/commit/f9508371b9f2a9106e8b6803ec8200affa44737f))
* Implement the initial Identity and Access Management (IAM) service with migrations, application logic, infrastructure, and gRPC delivery. ([96fe69f](https://github.com/mutugading/goapps-backend/commit/96fe69f34b0545fc6bd07b337e36ddf61812ee5d))


### Bug Fixes

* **email:** add SMTP client timeouts to prevent indefinite hanging ([020a81a](https://github.com/mutugading/goapps-backend/commit/020a81ae0a25ffca4b307d1ef4e41fa6a93e39c7))
* **email:** add SMTP client timeouts to prevent indefinite hanging ([331f073](https://github.com/mutugading/goapps-backend/commit/331f073c93f0fd9cf3632dcc2b6d750a81ebad25))
* **email:** handle errors when closing SMTP connection after SetDeadline failure ([9e1b2c3](https://github.com/mutugading/goapps-backend/commit/9e1b2c3b74ddeeafd904eb76fb14d59dc9f24325))
* **iam:** consolidate error mapping logic into mapUnknownError helper and include ErrEmailAlreadyVerified in conflict checks ([2f7d0ab](https://github.com/mutugading/goapps-backend/commit/2f7d0abf2263fc45863cc5a3a294c7e12f29a8b3))
* **iam:** implement email verification flow including database schema, cache storage, and gRPC service methods ([084bcac](https://github.com/mutugading/goapps-backend/commit/084bcac2d1f86c8cc28215d5ee503874a258ff0e))
* **iam:** implement email verification flow including database schema, cache storage, and gRPC service methods ([b1b0b3d](https://github.com/mutugading/goapps-backend/commit/b1b0b3d785b56b3ebc0c69c051dca02fba8bddfd))
* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([3387841](https://github.com/mutugading/goapps-backend/commit/3387841280e082eded628ce235998d5e4721eccf))
* lint check. ([84ef101](https://github.com/mutugading/goapps-backend/commit/84ef10161e9a600e55b3679c6c82cce33003eb2c))
* Seed raw material categories with IAM menu and permissions, and refactor sort order strings to constants. ([1330c63](https://github.com/mutugading/goapps-backend/commit/1330c630b279efc4302ca31758489ba44f712ad3))

## [0.8.2](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.8.1...iam-service/v0.8.2) (2026-04-14)


### Bug Fixes

* **iam:** consolidate error mapping logic into mapUnknownError helper and include ErrEmailAlreadyVerified in conflict checks ([2f7d0ab](https://github.com/mutugading/goapps-backend/commit/2f7d0abf2263fc45863cc5a3a294c7e12f29a8b3))
* **iam:** implement email verification flow including database schema, cache storage, and gRPC service methods ([084bcac](https://github.com/mutugading/goapps-backend/commit/084bcac2d1f86c8cc28215d5ee503874a258ff0e))
* **iam:** implement email verification flow including database schema, cache storage, and gRPC service methods ([b1b0b3d](https://github.com/mutugading/goapps-backend/commit/b1b0b3d785b56b3ebc0c69c051dca02fba8bddfd))

## [0.8.1](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.8.0...iam-service/v0.8.1) (2026-04-14)


### Bug Fixes

* **email:** add SMTP client timeouts to prevent indefinite hanging ([020a81a](https://github.com/mutugading/goapps-backend/commit/020a81ae0a25ffca4b307d1ef4e41fa6a93e39c7))
* **email:** add SMTP client timeouts to prevent indefinite hanging ([331f073](https://github.com/mutugading/goapps-backend/commit/331f073c93f0fd9cf3632dcc2b6d750a81ebad25))
* **email:** handle errors when closing SMTP connection after SetDeadline failure ([9e1b2c3](https://github.com/mutugading/goapps-backend/commit/9e1b2c3b74ddeeafd904eb76fb14d59dc9f24325))

## [0.8.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.7.0...iam-service/v0.8.0) (2026-04-13)


### Features

* add UOM category management with CRUD, import/export, and seed migration ([29e4c43](https://github.com/mutugading/goapps-backend/commit/29e4c432fdaf2ac13f81dd548488ed6d5596c75f))
* add UOM category management with CRUD, import/export, and seed migration ([f4cbaaa](https://github.com/mutugading/goapps-backend/commit/f4cbaaaed866fec031f8a0af05bdb98970d99254))

## [0.7.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.6.0...iam-service/v0.7.0) (2026-04-08)


### Features

* implement formula management service with CRUD operations, gRPC definitions, and database schema ([ee9831c](https://github.com/mutugading/goapps-backend/commit/ee9831c156013d0b81b3634645934392dffef324))
* implement formula management service with CRUD operations, gRPC definitions, and database schema ([1faa10c](https://github.com/mutugading/goapps-backend/commit/1faa10ce43da4772131957ef9f53ce95987e5038))

## [0.6.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.5.0...iam-service/v0.6.0) (2026-04-07)


### Features

* Add build and copy steps for the `iam-seed` binary. ([b55b3f8](https://github.com/mutugading/goapps-backend/commit/b55b3f89c6b4a66511a944ae7ad1ec9933793110))
* Add golang-migrate and migration files to the IAM service Dockerfile for database migrations. ([1de4a14](https://github.com/mutugading/goapps-backend/commit/1de4a147c48dc90e56e40c0fd0e1ee4f00834e05))
* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([5eb8515](https://github.com/mutugading/goapps-backend/commit/5eb8515c646e89a934bbd5694c4b9833c6accad8))
* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([c0df3b4](https://github.com/mutugading/goapps-backend/commit/c0df3b47eb36a97bf1eec792ac114ffa4d9571a8))
* implement finance parameter management module with CRUD, import/export, and UI components ([9505fd1](https://github.com/mutugading/goapps-backend/commit/9505fd10daa3aedf88a101e1aea7d0dfe572bcd3))


### Bug Fixes

* lint check. ([84ef101](https://github.com/mutugading/goapps-backend/commit/84ef10161e9a600e55b3679c6c82cce33003eb2c))
* Seed raw material categories with IAM menu and permissions, and refactor sort order strings to constants. ([1330c63](https://github.com/mutugading/goapps-backend/commit/1330c630b279efc4302ca31758489ba44f712ad3))

## [0.5.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.4.0...iam-service/v0.5.0) (2026-03-20)


### Features

* Add MinIO storage integration and user profile picture upload functionality. ([2754734](https://github.com/mutugading/goapps-backend/commit/27547346f3a6096fe3d4672a8fa7508f9835f2c3))
* Add MinIO storage integration and user profile picture upload functionality. ([8c8d7c0](https://github.com/mutugading/goapps-backend/commit/8c8d7c042e497002579b078fb57c6d2fb3ef9ff4))
* Implement session idle timeout, add comprehensive E2E tests, and include a backend run guide. ([5610f2c](https://github.com/mutugading/goapps-backend/commit/5610f2cc28d10f2c2a52db56fb24f539c98962a5))

## [0.4.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.3.0...iam-service/v0.4.0) (2026-02-09)


### Features

* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([55c70c8](https://github.com/mutugading/goapps-backend/commit/55c70c82a410ec6dbc328a1f0c1bec7609c6b84d))


### Bug Fixes

* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([3387841](https://github.com/mutugading/goapps-backend/commit/3387841280e082eded628ce235998d5e4721eccf))

## [0.3.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.2.0...iam-service/v0.3.0) (2026-02-08)


### Features

* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([c82c258](https://github.com/mutugading/goapps-backend/commit/c82c258bd39279f31789b9635367b2e4cb34d19b))
* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([a1ad398](https://github.com/mutugading/goapps-backend/commit/a1ad3980e6594d80e4154b0ecf4dfcb8f42f09c1))

## [0.2.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.1.0...iam-service/v0.2.0) (2026-02-07)


### Features

* Implement the initial Identity and Access Management (IAM) serv… ([f950837](https://github.com/mutugading/goapps-backend/commit/f9508371b9f2a9106e8b6803ec8200affa44737f))
* Implement the initial Identity and Access Management (IAM) service with migrations, application logic, infrastructure, and gRPC delivery. ([96fe69f](https://github.com/mutugading/goapps-backend/commit/96fe69f34b0545fc6bd07b337e36ddf61812ee5d))
