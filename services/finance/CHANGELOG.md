# Changelog

## [0.13.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.12.0...finance-service/v0.13.0) (2026-06-12)


### Features

* **bi-service:** Enhance BI module with migrations, dashboard features, and gRPC support ([060b249](https://github.com/mutugading/goapps-backend/commit/060b2495db306f617b04385f1f68bdbecc483f17))
* **bi:** add 11 BI table/MV migrations + 4 seed migrations (000300-000314) ([5fd8d21](https://github.com/mutugading/goapps-backend/commit/5fd8d21d4b00fac61fc7d90e6e947d73161823b6))
* **bi:** add available_chart_types to existing dashboard configs ([d1407d7](https://github.com/mutugading/goapps-backend/commit/d1407d797f13530a04f4114d2baa4600d308f0dd))
* **bi:** add available_chart_types to NET_PROFIT secondary chart (000326) ([956d597](https://github.com/mutugading/goapps-backend/commit/956d5972e899a0bb1f617279f8680cdfe19549bd))
* **bi:** add bi_metric_registry table with MIS + SALES metric seeds ([44ee8ec](https://github.com/mutugading/goapps-backend/commit/44ee8ecc621c01b582dffaad62f5952ac6076124))
* **bi:** add CronTrigger method to TriggerHandler for CRON:SYSTEM labelling ([c360d47](https://github.com/mutugading/goapps-backend/commit/c360d47be1442dbc1712286c7210c0923fc9dda5))
* **bi:** add CronTriggerer interface + scheduler unit tests ([0b9a6f1](https://github.com/mutugading/goapps-backend/commit/0b9a6f1ce48ffe0caa5b35b15781a24e0bf0c741))
* **bi:** add drill_enabled flags to secondary chart layout_config (000337) ([f72107b](https://github.com/mutugading/goapps-backend/commit/f72107bf43400303b935ee5c0cec8fbb82668b49))
* **bi:** add tooltip labels for Margin % by Category rich tooltip ([e5577d4](https://github.com/mutugading/goapps-backend/commit/e5577d4a8f972cb6bf7a9069217b0be1b13186b7))
* **bi:** application-layer handlers (CRUD + chartdata + datasource + job) ([19a1c5d](https://github.com/mutugading/goapps-backend/commit/19a1c5d312a64069a21631a26368c278ddeb58c3))
* **bi:** bi_job seeds for 3 ETL MV jobs (migration 000351) ([39ace4b](https://github.com/mutugading/goapps-backend/commit/39ace4b104c6ec8921c12e64b51bbb10bbdc42d8))
* **bi:** chart sub-package — registry, number-format, compare helpers ([79e83a8](https://github.com/mutugading/goapps-backend/commit/79e83a8ebdc00f71fd89143e218e92abdce2995f))
* **bi:** computed_ratio query plan for Margin % by Category secondary chart ([0ecc611](https://github.com/mutugading/goapps-backend/commit/0ecc611be5f375cf1c099a61f11c3aae1101695d))
* **bi:** cross_ratio KPI agg type for NP Margin vs EBITDA — computeCrossRatioKPI ([75ced6c](https://github.com/mutugading/goapps-backend/commit/75ced6cf7c5e99114632791a05221e413e0b66c8))
* **bi:** Dashboard aggregate root + Repository interface ([0d395fe](https://github.com/mutugading/goapps-backend/commit/0d395feed50034d5ab77fec751e320827d844fe5))
* **bi:** Dashboard value objects + sentinel errors ([122236a](https://github.com/mutugading/goapps-backend/commit/122236a86f2c1724615ea06df04a2bef7b558935))
* **bi:** DELIVERY_MARGIN 4 KPIs with metric_name filter + view_configs (000335) ([2b47f9b](https://github.com/mutugading/goapps-backend/commit/2b47f9bec015c6bcb35dcb1f1ce4276ad686ca10))
* **bi:** DELIVERY_MARGIN secondary cards get available_chart_types for view type switcher (000344) ([7c27950](https://github.com/mutugading/goapps-backend/commit/7c2795038e29547e526f64a1a726714009088efe))
* **bi:** DM 5th line (SELLING_COST) + EBITDA YTD KPI follows selected month ([936d524](https://github.com/mutugading/goapps-backend/commit/936d52449aa591761c6b70812f262a33f8383d55))
* **bi:** EBITDA dashboard — 4 KPI cards + view_configs per chart type + 2 secondary charts (000332-000333) ([b708686](https://github.com/mutugading/goapps-backend/commit/b708686f6c09727a215e11a54201d1fc6cb910ad))
* **bi:** enhance KPI configs — compare modes + sparklines all cards + 5th KPI for DELIVERY_MARGIN (000343) ([a3d622c](https://github.com/mutugading/goapps-backend/commit/a3d622cef558a3150c57a7e270747bb35172cb26))
* **bi:** ETL Job CRUD backend — Create/Update/Delete handlers + gRPC wiring ([a803670](https://github.com/mutugading/goapps-backend/commit/a803670a7a90c2fa8c01376e518743daa7b3bd2f))
* **bi:** Executive Dashboard landing — featured dashboards + pin/unpin (migration 000345) ([cb16bca](https://github.com/mutugading/goapps-backend/commit/cb16bca3297f2e2cfbf8fc1168211eaca227008d))
* **bi:** filter chips backend — group_1/group_2 WHERE filtering in planMultiMetric ([56f3f95](https://github.com/mutugading/goapps-backend/commit/56f3f95410d6dd3ede34d73ba3c320d7bda14678))
* **bi:** fix chart compare-overlay + application-layer tests ([#1](https://github.com/mutugading/goapps-backend/issues/1), [#2](https://github.com/mutugading/goapps-backend/issues/2)) ([4270780](https://github.com/mutugading/goapps-backend/commit/427078021a9f72352dd1803a09fe70f934e7ba82))
* **bi:** group, factmetric, datasource, job domain sub-packages ([e2493e3](https://github.com/mutugading/goapps-backend/commit/e2493e322639fcc7dad26c3ebfda014eb97ba3c1))
* **bi:** gRPC delivery handlers + wire 4 services in main.go + HTTP gateway ([25c3822](https://github.com/mutugading/goapps-backend/commit/25c38223d325b28e59fb8fe8953d9c2f9dc50779))
* **bi:** implement BiJobScheduler with overlap guard and 5-min DB sync ([32d2272](https://github.com/mutugading/goapps-backend/commit/32d2272ce72f26a58d432755e932639a0b50d794))
* **bi:** KpiEntry.MetricName — direct bi_fact_metric query for multi-metric SALES KPIs ([7029053](https://github.com/mutugading/goapps-backend/commit/702905390b35970e6f678f0531b111627a87a361))
* **bi:** landing_sections config for featured dashboards (mig 000349) ([4cd552a](https://github.com/mutugading/goapps-backend/commit/4cd552a088ab9abc02ad6de73b3ff788eacc014d))
* **bi:** multi-metric query planner — UNION ALL per metric for SALES dashboards ([6b6f08f](https://github.com/mutugading/goapps-backend/commit/6b6f08f29a703a78045675584bad4b890a6abcb3))
* **bi:** MV_REFRESH job handler — skips Oracle fetch, only refreshes materialized views ([58ec814](https://github.com/mutugading/goapps-backend/commit/58ec8147fe33e7667e496a5a5ed5d26722eee025))
* **bi:** MVLoader application handler for Oracle MV ETL ([1462202](https://github.com/mutugading/goapps-backend/commit/14622023e638ad6451f4e70f8353d02653f210b5))
* **bi:** NET_PROFIT dashboard — 4 KPIs (incl. cross_ratio NP Margin%) + view_configs (000334) ([2360294](https://github.com/mutugading/goapps-backend/commit/236029465a67cfd815fec0c05dd62691e1472f80))
* **bi:** NET_PROFIT secondary chart → dual_line with source_dashboard_code=EBITDA (000328) ([f2e671d](https://github.com/mutugading/goapps-backend/commit/f2e671d719f2ed9f7b9eefc86cc0fa841e79ec1d))
* **bi:** Oracle MV repository for BI dashboards (MIS/DELMAR/SALES) ([fae33b1](https://github.com/mutugading/goapps-backend/commit/fae33b1cdb8ca8c5230390ed10904520bb24911b))
* **bi:** planComputedRatio supports group_by field + empty denominator (single-metric aggregation); DELIVERY_MARGIN 3-chart secondary layout (000341) ([e1b953b](https://github.com/mutugading/goapps-backend/commit/e1b953b20f9e52517006a72feb72c581edce4863))
* **bi:** postgres repositories + redis chart cache ([1e927b3](https://github.com/mutugading/goapps-backend/commit/1e927b343c4b91ccd136f1bba1dd9343c7348edd))
* **bi:** real ERP seed, Excel upload, config audit, ETL seed + viewer fixes ([e73d480](https://github.com/mutugading/goapps-backend/commit/e73d48014197fd94385a6dfe7f77618595f12330))
* **bi:** rebuild MVs with metric_name grouping + agg_method=SUM filter (v1.1) ([f69ed9c](https://github.com/mutugading/goapps-backend/commit/f69ed9c9e937b6a674db3029797cd445b1fc09fb))
* **bi:** schema v1.1 — add metric_name/category/agg_method to bi_fact_metric ([0400113](https://github.com/mutugading/goapps-backend/commit/04001139d9bcd3577f11d4303720663b66380d2e))
* **bi:** seed delivery margin data (3,114 rows, SALES type, 6 metrics per combo) ([c1ce33c](https://github.com/mutugading/goapps-backend/commit/c1ce33c00f2e944fcaa796039485a4d100607db9))
* **bi:** seed DELIVERY_MARGIN dashboard config + IAM menu/permissions (000324+000046) ([ab4dcef](https://github.com/mutugading/goapps-backend/commit/ab4dcefdc7bdb9c569697aeb991d40eed8c99299))
* **bi:** shape multi-metric AggRows into separate Series per metric ([a25a7bd](https://github.com/mutugading/goapps-backend/commit/a25a7bdfe3b7137f2ea62adcdb64c68595d263d8))
* **bi:** typed ChartConfig + KpiConfig with registry-driven validation ([1c1b993](https://github.com/mutugading/goapps-backend/commit/1c1b9936d8fb76399616a06e44d486b326f5a1b5))
* **bi:** update EBITDA+NET_PROFIT layout_config for component_detail_table and monthly_detail_table ([0335f56](https://github.com/mutugading/goapps-backend/commit/0335f56e57a5db917534c935674465258241fada))
* **bi:** update FactMetric struct + Upsert for schema v1.1 metric_name fields ([e34f9eb](https://github.com/mutugading/goapps-backend/commit/e34f9ebbade88aaec62f0300c050760b5913b8ad))
* **bi:** ViewModeConfig domain type — per-view title/drill/hint + Dashboard.ViewConfigFor() ([b746b55](https://github.com/mutugading/goapps-backend/commit/b746b554f9d382edfd599348a81b18af7784dfd8))
* **bi:** wire BiJobScheduler + auto cache-invalidation after ETL/MV_REFRESH ([c632e63](https://github.com/mutugading/goapps-backend/commit/c632e63a123ac3eabe5ec385d178e330bbb26226))
* **bi:** wire MVLoader into TriggerHandler (kind=etl_mis/etl_delivery_margin/etl_sales) ([e75fe02](https://github.com/mutugading/goapps-backend/commit/e75fe02867355706ac5e15fc8e5828b4c67ddf90))
* **costing:** Cost Fill Assignments, CPR Workflow, and IAM Notification Redesign ([5399842](https://github.com/mutugading/goapps-backend/commit/5399842feed2de8951891db737d72f877e13e3c0))
* **finance/costcalc:** add 10 query + state-transition handlers ([f52b4ef](https://github.com/mutugading/goapps-backend/commit/f52b4efe3507c3708c0da10720af3f9ab691da74))
* **finance/costcalc:** emit audit events at job lifecycle boundaries ([ee91cf4](https://github.com/mutugading/goapps-backend/commit/ee91cf4d44ac84cb94a24f5b5306d451af108d64))
* **finance/db:** add migrations 000368, 000369, 000371 for fill config seed and CPR status extensions ([803734c](https://github.com/mutugading/goapps-backend/commit/803734c2eaebc888dffbe5aef5c13c4967b07fb0))
* **finance:** add aud_cost_history audit table ([1d715fa](https://github.com/mutugading/goapps-backend/commit/1d715fa0ea439e12e651a29d6c70334b4100af61))
* **finance:** add cal_job table + job code generator ([642dde3](https://github.com/mutugading/goapps-backend/commit/642dde3d5cf98a175d2d2cc820401920ae34ed0c))
* **finance:** add cal_job_chunk table ([210e3d2](https://github.com/mutugading/goapps-backend/commit/210e3d2eb345a9b451493231d7c68897d8194806))
* **finance:** add cal_job_product table ([5ebbc20](https://github.com/mutugading/goapps-backend/commit/5ebbc203e721ef8dc07f45690966600cb97d7a06))
* **finance:** add ComputeProduct core engine ([1f4fa5a](https://github.com/mutugading/goapps-backend/commit/1f4fa5a21c826247c1a1f6aaabeb285fcf3ead8a))
* **finance:** add Confirm, Approve, Release domain transitions to CPR entity ([ceca302](https://github.com/mutugading/goapps-backend/commit/ceca302f165275f61b471a9d366f7a7a2667a070))
* **finance:** add cost calc bulk loaders + topo-sorted formulas ([f4ee39d](https://github.com/mutugading/goapps-backend/commit/f4ee39d28336f9e17395673e8b72e9d1d941b296))
* **finance:** add cost_fill_task + cost_fill_approval tables ([8faffa3](https://github.com/mutugading/goapps-backend/commit/8faffa3a5032556780f2ad17fadd1e0399277798))
* **finance:** add cost_level_assignment_config table ([cc82fbb](https://github.com/mutugading/goapps-backend/commit/cc82fbb6e89a5571b20d9a3eb777c19e47455381))
* **finance:** add cost_request_status_history table and requesthistory domain package ([1129480](https://github.com/mutugading/goapps-backend/commit/11294800afc7be704164e7bf792b9d7ae2a1fd1f))
* **finance:** add costcalc domain layer (entities + repos + DAG + wave planner) ([cb766b5](https://github.com/mutugading/goapps-backend/commit/cb766b579d233299874e3793d20e1747c5146aef))
* **finance:** add costfillassignment domain (value objects, resolver, task state machine, repo interfaces) ([5f69f8e](https://github.com/mutugading/goapps-backend/commit/5f69f8e80f8ae80c93caf5a0b000679d587aaa0c))
* **finance:** add CPR_COMMENT_ADDED and CPR_MENTIONED notification events with actor name; mentioned users receive individual notifications ([e744768](https://github.com/mutugading/goapps-backend/commit/e74476841cbde5253afaed57974c3f4b47f2aa01))
* **finance:** add cpr_wfl_instance_id column for IAM workflow wiring ([efe4114](https://github.com/mutugading/goapps-backend/commit/efe4114ee61001a70dbdec686f34f6d7698ec377))
* **finance:** add CPRNotifier interface and IAM-backed implementation for rule-based CPR notifications ([fcd1d47](https://github.com/mutugading/goapps-backend/commit/fcd1d47e419e850de8a1ff4947613d00204757d2))
* **finance:** add cst_product table migration ([a712797](https://github.com/mutugading/goapps-backend/commit/a712797794799c433833341f249e2202ce8c5d88))
* **finance:** add cst_product_cost table for calc engine ([125bda8](https://github.com/mutugading/goapps-backend/commit/125bda89b0e321400795ac1669938becf538294e))
* **finance:** add expr-lang evaluator + compile cache for cost formulas ([1e0f3f6](https://github.com/mutugading/goapps-backend/commit/1e0f3f664d3aa1621d0717a9e6eb144ceb0a6140))
* **finance:** add fill-assignment config CRUD application handlers ([5e7697b](https://github.com/mutugading/goapps-backend/commit/5e7697b031c4aa60254315d807ca18e027d89da6))
* **finance:** add fill-assignment tracking query, overrides, SLA cron, resolver ([40720e7](https://github.com/mutugading/goapps-backend/commit/40720e708c31892d309da6e23bce7eb04f6da906))
* **finance:** add fill-task lifecycle application handlers ([d64288c](https://github.com/mutugading/goapps-backend/commit/d64288c5552a87c1e23b620072e5f9bdce16fc11))
* **finance:** add FillEventNotifier interface and IAM implementation; fix DEPT fill task notification gap ([718d459](https://github.com/mutugading/goapps-backend/commit/718d4599bdd329bc8137ca7691c42741e85ba67c))
* **finance:** add fk cpc_job_id -&gt; cal_job ([02cb1ef](https://github.com/mutugading/goapps-backend/commit/02cb1efbc0af66ec4e4e6693f087aad25b50f37d))
* **finance:** add IAM workflow client seam + best-effort Submit wiring ([a74c90a](https://github.com/mutugading/goapps-backend/commit/a74c90a34a04ba7c2c6c81dbd1ec1a89ace80afb))
* **finance:** add postgres fill-config + fill-task repositories ([9e71ad8](https://github.com/mutugading/goapps-backend/commit/9e71ad89916c54c4a49751e1ea103fb1832210ad))
* **finance:** add postgres repositories for cost calc engine ([0083b53](https://github.com/mutugading/goapps-backend/commit/0083b53048b1319273319bd2636402e819d852b6))
* **finance:** add prd_request + sequence migration for product request tickets ([2d347dc](https://github.com/mutugading/goapps-backend/commit/2d347dc4140405a8c9b799736bf3e542ac6e235a))
* **finance:** add prdrequest application handlers (CRUD + assign/resolve/reject + search) ([4b24c25](https://github.com/mutugading/goapps-backend/commit/4b24c2530686d2c2b4e7d27f1601dfa2f90427c6))
* **finance:** add prdrequest domain layer (entity, value objects, ticket-no generator, tests) ([63c979f](https://github.com/mutugading/goapps-backend/commit/63c979fca7f153478a64119213a95d205475ecd9))
* **finance:** add prdrequest postgres repository + atomic ticket-no generator ([ac6c4f8](https://github.com/mutugading/goapps-backend/commit/ac6c4f810110c7e5bf93a42c48135ef2b4efed19))
* **finance:** add product + request level assignment override tables ([8b18227](https://github.com/mutugading/goapps-backend/commit/8b18227e065c06a241377e596bda883b11be5b52))
* **finance:** add product application handlers (CRUD + duplicate) ([c5f905c](https://github.com/mutugading/goapps-backend/commit/c5f905c251b1d092d6d1d24fff621fc8204fd4a8))
* **finance:** add product domain layer (entity, value objects, repository interface, tests) ([57f729f](https://github.com/mutugading/goapps-backend/commit/57f729f863553867f0619bc3de3252913be98c41))
* **finance:** add product postgres repository with FTS search ([3d0ac55](https://github.com/mutugading/goapps-backend/commit/3d0ac556196a6d864d2ce7dc01aea07c956ed2a5))
* **finance:** add RequestNotification method to iamclient.NotificationClient ([a059178](https://github.com/mutugading/goapps-backend/commit/a059178e0aee385b54ee5042a4c57833a455b6e2))
* **finance:** commit costing feature migrations (000100-000226) ([485f38a](https://github.com/mutugading/goapps-backend/commit/485f38a239e7b25711b0a606a3dab7f6c4442c46))
* **finance:** CreateRouteFromProduct RPC + handler wiring ([ecabc8b](https://github.com/mutugading/goapps-backend/commit/ecabc8b1c56b23f9ec9863149c346238f9aba20e))
* **finance:** DuplicateRoute deep-fork + ListLinkedRequests ([c14ba51](https://github.com/mutugading/goapps-backend/commit/c14ba51f39cd3e7887d72b9c286e9fe5b703f684))
* **finance:** emit CPR_COMMENT_ADDED notification on new comment ([875e32f](https://github.com/mutugading/goapps-backend/commit/875e32f2bf64fde635dd369e27357abf72a2a405))
* **finance:** expand textile formulas (+9 formulas) ([f935fc8](https://github.com/mutugading/goapps-backend/commit/f935fc8873cfa5c80f5994b1da39b47d0dd97c66))
* **finance:** expand textile params catalog (+80 params) ([0218f7c](https://github.com/mutugading/goapps-backend/commit/0218f7cfddda1c33be4a654cfd5a6fd3efe2d4a3))
* **finance:** gate CostCalcService RPCs with permissions ([d6708da](https://github.com/mutugading/goapps-backend/commit/d6708da051f6a8ffdfe05bfca86c4b999c265216))
* **finance:** guard Task.Submit() against incomplete fills (Task 5 A3) ([be8a273](https://github.com/mutugading/goapps-backend/commit/be8a273ac798897c52c86db839d4a18ca728443f))
* **finance:** hook FillTaskCreator into MarkParameterPending ([825e912](https://github.com/mutugading/goapps-backend/commit/825e9123631808533eb30e162d88ee8d501c3728))
* **finance:** LinkExistingRoute + UnlinkRoute RPCs + handler arity widened ([1367bfe](https://github.com/mutugading/goapps-backend/commit/1367bfe60d0768b448f0cb728fb9811c79ddb859))
* **finance:** ListCostResults — cross-product cost list with resolved labels ([4b239ab](https://github.com/mutugading/goapps-backend/commit/4b239ab57a91adaf323e5a1b7d4a45afca81287a))
* **finance:** per-level fill task support — per-level param totals, staged activation, task notifications ([396fe75](https://github.com/mutugading/goapps-backend/commit/396fe75bbafeced0b283815b83f56d48a4eab7a2))
* **finance:** publish job_triggered event for non-SINGLE_PRODUCT scopes ([0ad3537](https://github.com/mutugading/goapps-backend/commit/0ad3537f436eb41bf423cf2bf475f859536b90bd))
* **finance:** read user permissions from IAM Redis cache in auth interceptor ([976d111](https://github.com/mutugading/goapps-backend/commit/976d111d558ca36f38b0cceacde1a652a49a5707))
* **finance:** remove L100-102 completion chain; mark parameter complete directly after all fills approved ([011761e](https://github.com/mutugading/goapps-backend/commit/011761e7e5759ec0d6713e42008349c05811b4d6))
* **finance:** Reopen transition for CLOSED product requests ([0a2f392](https://github.com/mutugading/goapps-backend/commit/0a2f392ffe9720209778cade765d023a673a7851))
* **finance:** request domain LinkRoute/UnlinkRoute + persist linked_route_head_id ([9bc2243](https://github.com/mutugading/goapps-backend/commit/9bc224376600a5e6b2b626e6fb26b50eb35d75e5))
* **finance:** revamp textile fixture -- deeper DAG + multi-stage routes + remap ITEM RMs ([9a44fdd](https://github.com/mutugading/goapps-backend/commit/9a44fddf8fbff556d9c0fcf6df91aea5a52ad0e4))
* **finance:** seed fill-assignment global configs + test task data ([5f7be85](https://github.com/mutugading/goapps-backend/commit/5f7be8513985d2368c105699f92ce34979137d94))
* **finance:** seed realistic textile products + DAG routes + CAPP (S8e-fix 3/3) ([4e8db7f](https://github.com/mutugading/goapps-backend/commit/4e8db7fa665c33a6f997fbec1c545eabd4fbc92a))
* **finance:** seed textile cost formulas (S8e-fix 2/3) ([6534c3f](https://github.com/mutugading/goapps-backend/commit/6534c3f2a5136e89a0bce224eec72a32d03b3c08))
* **finance:** seed textile master parameters catalog (S8e-fix 1/3) ([b1d0a1c](https://github.com/mutugading/goapps-backend/commit/b1d0a1c2c11c74b4eb2dc7e428afa962d3081dac))
* **finance:** stub CostCalcService handlers + wire into gRPC + gateway ([3570a73](https://github.com/mutugading/goapps-backend/commit/3570a73596841c23011edb2e2209980632d8a77b))
* **finance:** wire approval trace history into TransitionHandler; add GetCostProductRequestHistory gRPC handler ([c875ae0](https://github.com/mutugading/goapps-backend/commit/c875ae0d7bc348db81000c3c3bb2cfb56740ae4e))
* **finance:** wire CostCalcService end-to-end for single product calc ([f79fb8d](https://github.com/mutugading/goapps-backend/commit/f79fb8de93a67047275e15287b81468b08208f61))
* **finance:** wire CPR notification events in all state transitions; remove old AssignedToUserID-based notification ([7defd99](https://github.com/mutugading/goapps-backend/commit/7defd99651c2b0d55172afe8da3bcd7fe130cc1f))
* **finance:** wire fill-assignment gRPC handlers into delivery + main ([c67d154](https://github.com/mutugading/goapps-backend/commit/c67d15495a0cf327bfb9a4f3fea1aac8a192356e))
* **finance:** wire IAM notification client and route CPR/fill events through IAM fan-out ([afc67a5](https://github.com/mutugading/goapps-backend/commit/afc67a51d4f510495a5e3e040ac3c38f91731c10))
* **finance:** wire ProcessChunk + TriggerJob single-product path ([e5837cf](https://github.com/mutugading/goapps-backend/commit/e5837cf56da5f4962f1a88f364b91763309a2cdb))
* **finance:** wire product + prdrequest gRPC + REST gateway + DI ([2bf7bd5](https://github.com/mutugading/goapps-backend/commit/2bf7bd52e4fc18977e804b8cfd7f50aba6332a01))
* **finance:** wire SLA + fill-assignment reminder notifications to cron scheduler ([e2d09d3](https://github.com/mutugading/goapps-backend/commit/e2d09d30c253885f87a2f9d7f319db6ee1e6d34b))
* **phase-c:** S8e observability + cron + stress fixture ([a4489ff](https://github.com/mutugading/goapps-backend/commit/a4489ffcf5797aea480ea43ce0e77cd6784af16c))
* **phase-c:** S8e.5 distributed tracing across cost calc engine ([f287d23](https://github.com/mutugading/goapps-backend/commit/f287d23639c4ec703cfc1a61faa9ee63a5f49860))
* **product-cost:** Implement product costing system with migrations, handlers, and services ([173ddc6](https://github.com/mutugading/goapps-backend/commit/173ddc60c61e280da79ef78401ce0e73dd207a80))
* **proto+finance:** add ProcessChunkInternal RPC for worker bridge ([3d20e55](https://github.com/mutugading/goapps-backend/commit/3d20e550c1250a3c0825803a532ef7bae063e24e))


### Bug Fixes

* **bi-dashboard:** DELIVERY_MARGIN chart metrics and improve label configurations ([9ace628](https://github.com/mutugading/goapps-backend/commit/9ace6281c10361795b679cb8764d874133d0d586))
* **bi-dashboard:** Enhance BI metrics and dashboards with schema v1.1 updates ([bbb088d](https://github.com/mutugading/goapps-backend/commit/bbb088d8cd7c199f936113f11731506dd76a711c))
* **bi:** add bar chart type to Net Sales by Delivery Type card (D3) ([9551d95](https://github.com/mutugading/goapps-backend/commit/9551d9504a4f183c3ef016d280ecc6e33dc4eb53))
* **bi:** add chip labels to DELIVERY_MARGIN + align monthly_detail metric name ([c13f743](https://github.com/mutugading/goapps-backend/commit/c13f7439f31b3f771a96b17fe4bc4624dd2ab3c1))
* **bi:** add FilterChipsGroup1/2 to ChartConfig struct + MarshalToMap so filter chips reach frontend (D1) ([46cf3e2](https://github.com/mutugading/goapps-backend/commit/46cf3e213d227aa1617d2d0c038a436f359f8948))
* **bi:** add json tags to ComputedRatioConfig so group_by from BFF correctly maps to GroupBy (D3) ([af814b4](https://github.com/mutugading/goapps-backend/commit/af814b48ddb05f95e687fec7302cbc5f4e17ffb4))
* **bi:** add line/area type switcher to Net Profit vs EBITDA chart (C) ([c7aa02d](https://github.com/mutugading/goapps-backend/commit/c7aa02dc64e3505c3dff7c28d325eeb709da7bc6))
* **bi:** add period scoping (current_month/ytd/l12m) to all dashboard KPI configs (000342) ([543ccb4](https://github.com/mutugading/goapps-backend/commit/543ccb419ddd7199a7ca3360ee480150faf44491))
* **bi:** add selected_ytd to allowedKpiPeriods whitelist ([f77f0e8](https://github.com/mutugading/goapps-backend/commit/f77f0e81a25c064296bcfaccf18976e6f8e87931))
* **bi:** add tooltip labels for Margin % by Category rich tooltip ([e8ee9e7](https://github.com/mutugading/goapps-backend/commit/e8ee9e7e753aa46b138d0f22a8ee71ede6dfbf9f))
* **bi:** correct DELIVERY_MARGIN chart by adding metric_filter ([3624254](https://github.com/mutugading/goapps-backend/commit/36242546308959495276930b0c1c2502190536df))
* **bi:** delivery margin chart — 4 core USD metrics per UX design ([424e5b6](https://github.com/mutugading/goapps-backend/commit/424e5b670ec88863aa008ef7eb61caa9a366eb7c))
* **bi:** exclude Oracle pre-computed total rows from Postgres MVs ([ec060af](https://github.com/mutugading/goapps-backend/commit/ec060af1882a74162ca598f68bfb46d049039eb4))
* **bi:** extract mv_bi_metric_g1 string to const for goconst linter ([3708a6b](https://github.com/mutugading/goapps-backend/commit/3708a6b4cfcfdccd6eb222e822090394edac2c0c))
* **bi:** finalize migration 000358 — correct array format + remove explicit periods ([5a0c0ae](https://github.com/mutugading/goapps-backend/commit/5a0c0ae048aa827df6cbc7bb7b9dc54dcdeced3b))
* **bi:** invalidate Redis chart cache after MV_REFRESH and ETL success ([a5da6ec](https://github.com/mutugading/goapps-backend/commit/a5da6ec259854a059b1368e8108a2a2daa55d8f6))
* **bi:** KPI widgets respect Group1/Group2 filter-chip selections (viewer filter propagated to ComputeKPIs) ([de97f95](https://github.com/mutugading/goapps-backend/commit/de97f95f9c45c3001f60892660a75a28b555366f))
* **bi:** NET_PROFIT cross_ratio KPI — remove scale=100, percent format handles ×100 (000336) ([435beee](https://github.com/mutugading/goapps-backend/commit/435beee9dc10c951f2f74db8f4d5fd53a23cdda6))
* **bi:** pass job.SourceID to ETL Load() so FK constraint is satisfied ([86fd220](https://github.com/mutugading/goapps-backend/commit/86fd220180d5576fb74870b41d853c23e5a7d120))
* **bi:** pass job.SourceID to ETL Load() so FK constraint is satisfied ([077cae1](https://github.com/mutugading/goapps-backend/commit/077cae116aeb5c9beb43e547848db32b7d54cf59))
* **bi:** planComputedRatio respects Group1/Group2 filter chip selections (D2) ([c31a165](https://github.com/mutugading/goapps-backend/commit/c31a16503c2ccc136d3ce68f4da3d418980a3480))
* **bi:** planMultiMetric series inversion + force-trend for monthly-detail ([ddf85b7](https://github.com/mutugading/goapps-backend/commit/ddf85b7b72c6f63206db3a42dffd5018d665b0c9))
* **bi:** preserve available_chart_types in ChartConfig through parse/marshal cycle ([b3f2e42](https://github.com/mutugading/goapps-backend/commit/b3f2e42d236c09418538045178b7366bfb2fca15))
* **bi:** preserve kpi_config items wrapper in migration 000358 ([ec1e09a](https://github.com/mutugading/goapps-backend/commit/ec1e09a1b066ccf599f4ed1bf4755cb89dcb4e75))
* **bi:** QUANTITY UOM PCS→KGS (000330) + seed ETL_DELIVERY_MARGIN job (000331) ([b539797](https://github.com/mutugading/goapps-backend/commit/b539797071ec4ed2f351ef4a3333e01ce2cdd163))
* **bi:** remove unused sourceCodeUUID function ([49c5a01](https://github.com/mutugading/goapps-backend/commit/49c5a01b6ab6f0886a150dde90ed424a91a72c80))
* **bi:** rename delivery margin type SALES→DELIVERY MARGIN in fact data + dashboard (migration 000350) ([9cabd5f](https://github.com/mutugading/goapps-backend/commit/9cabd5f0592b9ad5f8689bcf5e5a976e92cb8c34))
* **bi:** replace non-ASCII dashes in kpi_compute.go comments (gofmt) ([decd2ad](https://github.com/mutugading/goapps-backend/commit/decd2ad63119dffeee508ae9fdbbcd38ff58010d))
* **bi:** resolve nestif + unconvert lint errors in job handlers/scheduler ([0fdb41a](https://github.com/mutugading/goapps-backend/commit/0fdb41a17fb528801f9f8d93fc3a7a4e9e8d77e6))
* **bi:** restore EBITDA available_chart_types in chart_config (000338) ([ed1b1db](https://github.com/mutugading/goapps-backend/commit/ed1b1dbbf5301c5d198542f5d74879ee18f45a99))
* **bi:** scheduler detects cron expression changes without deactivate cycle ([b636494](https://github.com/mutugading/goapps-backend/commit/b6364948e82c55d45bcbb8e90b3b41a5445837a1))
* **bi:** skip group_2 filter on mv_bi_metric_g1 in KPI computation ([3259f37](https://github.com/mutugading/goapps-backend/commit/3259f37a37b057b109ea64176f5e95dc8e5b3e7a))
* **bi:** skip group_2 filter on mv_bi_metric_g1 in KPI computation ([3d127f0](https://github.com/mutugading/goapps-backend/commit/3d127f0d597b873846e3e89cc8c4307012d4fc39))
* **bi:** store static filter chip values for DELIVERY_MARGIN (D1, mig 000348) ([ab758cb](https://github.com/mutugading/goapps-backend/commit/ab758cbf6efd4f83162997d5b69bdcf488589dbe))
* **bi:** UNIQUE NULLS NOT DISTINCT on fact business key + integration test ([#3](https://github.com/mutugading/goapps-backend/issues/3)) ([f9d5687](https://github.com/mutugading/goapps-backend/commit/f9d5687ce0a0ab2925194ae1d1ac839867ad2f29))
* **bi:** use min() builtin for batch end bound in MVLoader ([d48c357](https://github.com/mutugading/goapps-backend/commit/d48c357ed3793d5360dd3d82fb4b68989dd62d2a))
* **bi:** use string chart type in migration 000352, not proto enum int ([6721153](https://github.com/mutugading/goapps-backend/commit/6721153694f7857d83a6a01fdf7834f9edfefcf8))
* chart_type = 'line' (up) / chart_type = 'stacked_bar' (down). ([6721153](https://github.com/mutugading/goapps-backend/commit/6721153694f7857d83a6a01fdf7834f9edfefcf8))
* **ci:** errcheck blank type assertions + gofmt in handlers.go — use configString helper ([8efd7ff](https://github.com/mutugading/goapps-backend/commit/8efd7ff1fd5e67b6a1c19be6c1bad5b8b308eb92))
* **ci:** fix gofmt/goimports on server.go, main.go, cost_product_request_handler.go ([9aa8fe3](https://github.com/mutugading/goapps-backend/commit/9aa8fe39a7a42a78b3fb335526337a83ceaff7bf))
* **ci:** fix goimports local-prefix grouping across finance and iam ([7a73b25](https://github.com/mutugading/goapps-backend/commit/7a73b254c454ef94646514c95cba77822cec5d0e))
* **ci:** resolve all 6 golangci-lint errors + integration test missing metric_name column ([c4c5944](https://github.com/mutugading/goapps-backend/commit/c4c59443159a4774c601749072d02be6573849dd))
* **ci:** resolve golangci-lint and test failures on PR [#117](https://github.com/mutugading/goapps-backend/issues/117) ([0c9fd0f](https://github.com/mutugading/goapps-backend/commit/0c9fd0f8dca878a8f9e17c87b7948c7efef40e23))
* **ci:** resolve remaining gofmt and FilledAt hydration failures ([874f7b3](https://github.com/mutugading/goapps-backend/commit/874f7b35daaba66aeb7ff8f1d1c439786b7f8b6b))
* Dynamic YTD KPI for BI Dashboards & IAM Menu Permissions ([#115](https://github.com/mutugading/goapps-backend/issues/115)) ([69e9171](https://github.com/mutugading/goapps-backend/commit/69e9171597b06b47b512dd46b151ee1ed7cf51ab))
* **finance/costcalc:** broaden test cleanup to scoop all cst_rm_cost rows ([8f28eb6](https://github.com/mutugading/goapps-backend/commit/8f28eb62c7f1bc962167754d1c3d50e79bf500d9))
* **finance/costcalc:** LoadRoutesByProducts resolves intermediates via cost_route_seq ([6a2bf13](https://github.com/mutugading/goapps-backend/commit/6a2bf13973be845a033a8454904b3b15ce6e256c))
* **finance/costcalc:** route SINGLE_PRODUCT through orchestrator for upstream DAG walk ([b2e46c7](https://github.com/mutugading/goapps-backend/commit/b2e46c748f12b131cb1e7714e55b312198cd7297))
* **finance+seed:** unconditional auth bypass for ProcessChunkInternal + backfill default CAPP values ([6bcb362](https://github.com/mutugading/goapps-backend/commit/6bcb36263a69f56c6e53d5e32291f193944dda5f))
* **finance:** add Confirm/Approve/Release/MarkParamComplete permissions; sanitize history error response ([eb7b862](https://github.com/mutugading/goapps-backend/commit/eb7b8627e1c0fe88dbc07cc87a614b4b0eb81599))
* **finance:** address calc-engine migration review findings ([a3b02cd](https://github.com/mutugading/goapps-backend/commit/a3b02cdca892a0fba6721f2a185afa7658400433))
* **finance:** backfill CAPP for every active formula input + result sink ([4e2b26a](https://github.com/mutugading/goapps-backend/commit/4e2b26a2d7217eb0ea6240a528e14cde43efe24e))
* **finance:** backfill RATE-category CAPP values (LBR_RATE_TECH et al) ([981dbe9](https://github.com/mutugading/goapps-backend/commit/981dbe9b66a9d00146f75a7ac23ef01c40a91a1a))
* **finance:** COPY pkg/ in Dockerfile so go mod download resolves pkg/costcalc ([606bc00](https://github.com/mutugading/goapps-backend/commit/606bc00d8e7c0bcd07ff38af53e6789f6da33b33))
* **finance:** correct CPR_DRAFT_CREATED notification recipients and split creator ack ([206d43a](https://github.com/mutugading/goapps-backend/commit/206d43ad168f37d5100a10c30e8307aea90469f9))
* **finance:** drop incorrect per-product TXFX routes (prep for re-seed) ([17491ea](https://github.com/mutugading/goapps-backend/commit/17491ea13ee3c3bd670b7cdea883c56cb27ef983))
* **finance:** DuplicateRoute — buffer source rows before INSERT (bad connection bug) ([0b001ea](https://github.com/mutugading/goapps-backend/commit/0b001ea4f5612e104d4e452faa20f90114d40625))
* **finance:** exclude CALCULATED params from fill task total and progress counts ([b2a5774](https://github.com/mutugading/goapps-backend/commit/b2a5774d11297e71712e472922e01232d410aa31))
* **finance:** re-seed textile routes as self-contained multi-product DAGs ([ca75bf6](https://github.com/mutugading/goapps-backend/commit/ca75bf68bde4717e7109c2bf557857d1199817c3))
* **finance:** renumber duplicate migration 000365 → 000366/000367 ([bb4c019](https://github.com/mutugading/goapps-backend/commit/bb4c019f9525e94fdbea7a07cf4db3af9c06a47f))
* **finance:** resolve golangci-lint v2.3.0 CI failures (126 issues → 0) ([4e2bcda](https://github.com/mutugading/goapps-backend/commit/4e2bcda3820c718af88bc1284efa03862bf16a27))
* **finance:** thread actorName via applyOpts to fix history recording; add history permission entry ([0ff0226](https://github.com/mutugading/goapps-backend/commit/0ff022676ad32c2eeb40af548e0bf08705ecf73f))
* **finance:** use closeRows helper; replace standalone timestamp index with composite covering index ([8534f81](https://github.com/mutugading/goapps-backend/commit/8534f81259c4a2fd4c0b7f55f9f544a0ccf48ffb))
* **finance:** wire fillIAMNotifier into SubmitFillHandler so approver gets FILL_APPROVAL_PENDING notification ([c4d9e0f](https://github.com/mutugading/goapps-backend/commit/c4d9e0f1825e7a91c9ec4cf43c62cc443f1e2e81))
* **finance:** wrap 000233 in BEGIN/COMMIT to match sibling migration style ([21fb9ad](https://github.com/mutugading/goapps-backend/commit/21fb9ade69af08191b22f3e3ffbee8f4fb652617))
* lint sweep for fill-assignment — package comment + rows.Close errcheck ([a1bc9c9](https://github.com/mutugading/goapps-backend/commit/a1bc9c95ed79dad7429e3e0ba44c30c4d3b337a2))
* service-to-service auth for worker → finance via shared secret ([629e77e](https://github.com/mutugading/goapps-backend/commit/629e77e9d95057f47a6feb002ca1b5adfdb3a708))
* **test:** add metric_name to bi_fact_metric test UNIQUE constraint to match ON CONFLICT clause ([028aa27](https://github.com/mutugading/goapps-backend/commit/028aa2700ffa174301e18660261a1f5a09270fd6))

## [0.12.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.11.4...finance-service/v0.12.0) (2026-05-07)


### Features

* **export-notification:** Implement generic IAM notification system with MinIO export support ([124f619](https://github.com/mutugading/goapps-backend/commit/124f619c16e4f2ef5bc70c975fc9e40deb2e1244))
* **infra:** add MinIO storage client and IAM gRPC client wrapper ([4266c38](https://github.com/mutugading/goapps-backend/commit/4266c38f52de657555a1eb0cbaf973b1cb4a6578))
* **rm-cost:** async export to MinIO with EXPORT_READY notification emit ([3d46ba5](https://github.com/mutugading/goapps-backend/commit/3d46ba538af45513c4bb9cb8292c436cecc8ecf8))


### Bug Fixes

* **lint:** resolve golangci-lint errors and apply Copilot review feedback ([9b96fd5](https://github.com/mutugading/goapps-backend/commit/9b96fd5fc97b200577cca144b5d3656a6930f639))
* **lint:** resolve remaining gocyclo, gocognit, and errorlint failures ([962dcfd](https://github.com/mutugading/goapps-backend/commit/962dcfdad6700bb843643589396696dc7b4d4221))
* **s3:** presign against public endpoint to avoid signature mismatch ([e17a75a](https://github.com/mutugading/goapps-backend/commit/e17a75a078661e2cf29514ede49517b1790a169f))
* **storage:** presign against public endpoint to avoid signature mismatch ([287b46a](https://github.com/mutugading/goapps-backend/commit/287b46a267a65d716a04cfeaafb62f680e8f3a7b))
* **tracing:** fetch tracer lazily per-request to survive late provider init ([0186299](https://github.com/mutugading/goapps-backend/commit/018629984683afb9ba47c9f02ee198a36f835336))
* **tracing:** fetch tracer lazily per-request to survive late provider init ([788d0bc](https://github.com/mutugading/goapps-backend/commit/788d0bc6c0db634bd90430354542d21086b541a5))
* **tracing:** use otlptracegrpc.WithInsecure() to actually disable TLS ([0908955](https://github.com/mutugading/goapps-backend/commit/090895526959ecbc8deaa37a8f7fc1dc661aa8b7))

## [0.11.4](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.11.3...finance-service/v0.11.4) (2026-05-06)


### Bug Fixes

* **rm-cost:** document semantics of stock columns in cst_rm_cost_detail for clarity ([5874eb6](https://github.com/mutugading/goapps-backend/commit/5874eb697853391ad5ce50ee458d4ff1d60fa2bb))
* **syncdata:** enhance stock handling in FetchSourceQtyByItemGrade for accurate inventory reporting ([a0c5edd](https://github.com/mutugading/goapps-backend/commit/a0c5eddf769e16058770c2e7f55b1212abf443c0))

## [0.11.3](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.11.2...finance-service/v0.11.3) (2026-05-05)


### Bug Fixes

* **rm-cost:** implement V2 flag resolution for valuation and marketing in RMCost handler ([ef09750](https://github.com/mutugading/goapps-backend/commit/ef09750f2355d549bf4ea8a60fe481821ea6646f))
* **rm-cost:** implement V2 flag resolution for valuation and marketing in RMCost handler ([2afb1d1](https://github.com/mutugading/goapps-backend/commit/2afb1d1fcd650eb6c7a59d87a398dd7f9d5085d8))

## [0.11.2](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.11.1...finance-service/v0.11.2) (2026-05-05)


### Bug Fixes

* **ungrouped-handler:** optimize UngroupedQuery to UngroupedItemsFilter conversion by using direct type conversion ([b0e2a30](https://github.com/mutugading/goapps-backend/commit/b0e2a3078389f9efcd8559c9373db1906b3b233d))
* **ungrouped-items:** update UngroupedItems handling to support group ing monitor view with enhanced filtering and sorting options ([0611c25](https://github.com/mutugading/goapps-backend/commit/0611c250f41e2c75791f4e88f8f7e14c5f359f51))
* **ungrouped-items:** update UngroupedItems handling to support grouping monitor view with enhanced filtering and sorting options ([2909d73](https://github.com/mutugading/goapps-backend/commit/2909d73a2643811664609f1d533923a6f41b88b7))

## [0.11.1](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.11.0...finance-service/v0.11.1) (2026-05-04)


### Bug Fixes

* add search and explicit ID filters to ExportRMGroups and refactor RM Group import handler to support V2 schema ([5dbcfbd](https://github.com/mutugading/goapps-backend/commit/5dbcfbdbabcebb3663e2449d5df2f422a5f6846b))
* add search and explicit ID filters to ExportRMGroups and refactor RM Group import handler to support V2 schema ([76584a2](https://github.com/mutugading/goapps-backend/commit/76584a2ce0fc02636d2f199bf4bb92d20ef60546))
* improve error handling in RM group export and ensure proper resource cleanup in item lookup ([336c54d](https://github.com/mutugading/goapps-backend/commit/336c54d89c19f1577c4ecbc4b40e9f5789db9420))
* **test:** enhance ambiguity handling in RM group import tests for CI compatibility ([c0bc804](https://github.com/mutugading/goapps-backend/commit/c0bc8042ec2dc68deca9fbeadf6c7bfd61e0ba25))
* **test:** enhance ambiguity handling in RM group import tests for CI compatibility. ([144ba88](https://github.com/mutugading/goapps-backend/commit/144ba885642cba5dece9a9d96e2c6f88079d9ad1))

## [0.11.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.10.0...finance-service/v0.11.0) (2026-04-30)


### Features

* implement RM cost calculation v2 and expand raw material group data models with database migrations ([ffafed9](https://github.com/mutugading/goapps-backend/commit/ffafed9b5220e8b0888c522a98013569ebbf52ab))
* implement RM cost calculation v2 and expand raw material group data models with database migrations ([d8a9022](https://github.com/mutugading/goapps-backend/commit/d8a902271b371744f6cf42e01679e4a7d2d8a787))

## [0.10.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.9.1...finance-service/v0.10.0) (2026-04-22)


### Features

* implement raw material grouping and cost management modules with associated gRPC services and database migrations ([f67d111](https://github.com/mutugading/goapps-backend/commit/f67d111cb998323e80f8d3a8b9b93859227af4fa))
* implement raw material grouping and cost management modules with associated gRPC services and database migrations ([a24776a](https://github.com/mutugading/goapps-backend/commit/a24776a45003a72248a9c45c0d35dd776d23ada8))


### Bug Fixes

* centralize group head ID parsing and apply consistent formatting to colourant field labels ([cbeb463](https://github.com/mutugading/goapps-backend/commit/cbeb4630294606260fa63daab98579c32f904ea2))
* improve file handling, add linting annotations, and fix formatting across finance services ([365f51b](https://github.com/mutugading/goapps-backend/commit/365f51bd7c7885e5e4c4ef9d5ca8934241cee861))
* rename Colourant to Colorant throughout the codebase and update minor internal helpers ([25e8f5c](https://github.com/mutugading/goapps-backend/commit/25e8f5cc1cef2276107e42f9aab375634dbbb91f))

## [0.9.1](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.9.0...finance-service/v0.9.1) (2026-04-17)


### Bug Fixes

* **finance:** correct Oracle column mappings and update comments for consistency and accuracy ([8caee5d](https://github.com/mutugading/goapps-backend/commit/8caee5de40dd82417fa5040f09d95e93505b177b))
* **finance:** correct Oracle column mappings and update comments for consistency and accuracy ([6b87199](https://github.com/mutugading/goapps-backend/commit/6b871998cd18f34edfa8f45ab612d2e5fea7c91d))

## [0.9.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.8.1...finance-service/v0.9.0) (2026-04-17)


### Features

* **finance:** implement job execution tracking, Oracle sync, and RabbitMQ integration with migrations and handlers ([8f9896c](https://github.com/mutugading/goapps-backend/commit/8f9896c2de11dbb5428a71cb20430bec773a7a05))
* **finance:** implement job execution tracking, Oracle sync, and RabbitMQ integration with migrations and handlers ([02db90f](https://github.com/mutugading/goapps-backend/commit/02db90f9eee87a72611700192d10614266cd65b8))


### Bug Fixes

* **finance:** enhance Oracle sync system with improved error handling, concurrency safety, and refined data validation ([4ce67ee](https://github.com/mutugading/goapps-backend/commit/4ce67eeb620dd423b9dab6a77bfe52ad3c21c839))
* **finance:** standardize "canceled" spelling, update identifiers and comments to match proto/DB conventions ([3747f0a](https://github.com/mutugading/goapps-backend/commit/3747f0a154d62bc7558a394e241041b8d37b473a))
* **finance:** update Oracle sync procedure and schema, align period logic, and refine tests ([fe45bc0](https://github.com/mutugading/goapps-backend/commit/fe45bc07519e1ae19dded630c2c46c42304346e7))

## [0.8.1](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.8.0...finance-service/v0.8.1) (2026-04-15)


### Bug Fixes

* **chore:** add shared module copy step to Dockerfile iam and finance svc for dependency resolution ([ea159be](https://github.com/mutugading/goapps-backend/commit/ea159bee99c871929bc6dda7fc060a23c5431843))
* **chore:** add shared module copy step to Dockerfile iam and finance svc for dependency resolution ([a0e2c65](https://github.com/mutugading/goapps-backend/commit/a0e2c65578d70838ca78c6f3b833e2abd92bca22))

## [0.8.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.7.0...finance-service/v0.8.0) (2026-04-13)


### Features

* add UOM category management with CRUD, import/export, and seed migration ([29e4c43](https://github.com/mutugading/goapps-backend/commit/29e4c432fdaf2ac13f81dd548488ed6d5596c75f))
* add UOM category management with CRUD, import/export, and seed migration ([f4cbaaa](https://github.com/mutugading/goapps-backend/commit/f4cbaaaed866fec031f8a0af05bdb98970d99254))


### Bug Fixes

* **lint:** add nolint:dupl directives to category repositories and handlers ([035d9d1](https://github.com/mutugading/goapps-backend/commit/035d9d1343b0817174f562e9221f1271fce37909))
* **test:** update UOM E2E tests to use dynamic category IDs via UOMCategoryService ([6412c0f](https://github.com/mutugading/goapps-backend/commit/6412c0f97a18df609d932f933a2942ae053693cf))

## [0.7.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.6.0...finance-service/v0.7.0) (2026-04-08)


### Features

* implement formula management service with CRUD operations, gRPC definitions, and database schema ([ee9831c](https://github.com/mutugading/goapps-backend/commit/ee9831c156013d0b81b3634645934392dffef324))
* implement formula management service with CRUD operations, gRPC definitions, and database schema ([1faa10c](https://github.com/mutugading/goapps-backend/commit/1faa10ce43da4772131957ef9f53ce95987e5038))


### Bug Fixes

* enforce unique result parameters per formula and add validation constraints for description and input parameters ([5f8dea5](https://github.com/mutugading/goapps-backend/commit/5f8dea554ace536e7d949f7ac46c76ca1b1a19f5))
* update formula parsing methods to return change status alongside values ([b5e5d75](https://github.com/mutugading/goapps-backend/commit/b5e5d75abc5c228236589f4abdd5639942a38d35))

## [0.6.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.5.0...finance-service/v0.6.0) (2026-04-07)


### Features

* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([5eb8515](https://github.com/mutugading/goapps-backend/commit/5eb8515c646e89a934bbd5694c4b9833c6accad8))
* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([c0df3b4](https://github.com/mutugading/goapps-backend/commit/c0df3b47eb36a97bf1eec792ac114ffa4d9571a8))
* implement finance parameter management module with CRUD, import/export, and UI components ([9505fd1](https://github.com/mutugading/goapps-backend/commit/9505fd10daa3aedf88a101e1aea7d0dfe572bcd3))
* Implement Raw Material Category (RMCategory) management within the finance service. ([c10603a](https://github.com/mutugading/goapps-backend/commit/c10603a71e3760a309da86b58725f93c07d6b9d5))
* Introduce finance service seed and migrate jobs, and add an infrastructure stability guide. ([41fa0fc](https://github.com/mutugading/goapps-backend/commit/41fa0fcebbb2593fa0456be73039079a97be2a4d))


### Bug Fixes

* **finance:** resolve golangci-lint v2 errors for parameter module ([96996cf](https://github.com/mutugading/goapps-backend/commit/96996cf6fb3a2e4faaf6b12f57cbd0cb962b96f8))
* Seed raw material categories with IAM menu and permissions, and refactor sort order strings to constants. ([1330c63](https://github.com/mutugading/goapps-backend/commit/1330c630b279efc4302ca31758489ba44f712ad3))

## [0.5.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.4.0...finance-service/v0.5.0) (2026-03-20)


### Features

* Implement session idle timeout, add comprehensive E2E tests, and include a backend run guide. ([5610f2c](https://github.com/mutugading/goapps-backend/commit/5610f2cc28d10f2c2a52db56fb24f539c98962a5))

## [0.4.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.3.0...finance-service/v0.4.0) (2026-02-09)


### Features

* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([55c70c8](https://github.com/mutugading/goapps-backend/commit/55c70c82a410ec6dbc328a1f0c1bec7609c6b84d))


### Bug Fixes

* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([3387841](https://github.com/mutugading/goapps-backend/commit/3387841280e082eded628ce235998d5e4721eccf))

## [0.3.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.2.0...finance-service/v0.3.0) (2026-02-09)


### Features

* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([c82c258](https://github.com/mutugading/goapps-backend/commit/c82c258bd39279f31789b9635367b2e4cb34d19b))
* Implement JWT authentication in UOM E2E tests and update the CI workflow to provide the necessary secret. ([f3dac2a](https://github.com/mutugading/goapps-backend/commit/f3dac2a846178b0ebd80443b564b6182795c360b))
* Implement JWT authentication in UOM E2E tests and update the CI workflow to provide the necessary secret. ([a080253](https://github.com/mutugading/goapps-backend/commit/a080253986e0700c0ef996eb360ce5722273c406))

## [0.2.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.1.0...finance-service/v0.2.0) (2026-02-08)


### Features

* Add `ActiveFilter` enum for UOM queries, make `UpdateUOMRequest` fields optional, clarify `uom_code` immutability, and pin Makefile tool versions ([0b2c1ae](https://github.com/mutugading/goapps-backend/commit/0b2c1aeee96f150f60bffac2b3c17f059d05c1df))
* Add `ActiveFilter` enum for UOM queries, make `UpdateUOMRequest` fields optional, clarify `uom_code` immutability, and pin Makefile tool versions ([39f4bf3](https://github.com/mutugading/goapps-backend/commit/39f4bf3126c725aba80ee6de4b6300d2b3f5f11d))
* Add `test-ci-local` command for running integration tests and make audit log index creation idempotent. ([c64afe0](https://github.com/mutugading/goapps-backend/commit/c64afe0b5782a216d2b3a85f66ecf165a611213c))
* Embed `swagger.json` into the binary and serve it directly from memory. ([d691b45](https://github.com/mutugading/goapps-backend/commit/d691b45c3a2cd45da3070b9dbe78b020b304c9a4))
* Embed `swagger.json` into the binary and serve it directly from memory. ([638befc](https://github.com/mutugading/goapps-backend/commit/638befcb36556af3674279d2c34f5a81681b665b))
* Implement the initial Identity and Access Management (IAM) serv… ([f950837](https://github.com/mutugading/goapps-backend/commit/f9508371b9f2a9106e8b6803ec8200affa44737f))
* Implement the initial Identity and Access Management (IAM) service with migrations, application logic, infrastructure, and gRPC delivery. ([96fe69f](https://github.com/mutugading/goapps-backend/commit/96fe69f34b0545fc6bd07b337e36ddf61812ee5d))
* Implement the new finance service for Unit of Measurement (UOM) management with full CRUD, import/export, and infrastructure components. ([b8238c1](https://github.com/mutugading/goapps-backend/commit/b8238c1e207695fafd0f916e8ddcfb4c5a5d5caa))
* Implement the new finance service for Unit of Measurement (UOM) management with full CRUD, import/export, and infrastructure components. ([58c1cee](https://github.com/mutugading/goapps-backend/commit/58c1cee8c5763e4f5384909bcfb3e95f4c5bb10b))


### Bug Fixes

* **ci:** add `test-ci-local` command for running integration tests and make audit log index creation idempotent. ([1120a45](https://github.com/mutugading/goapps-backend/commit/1120a453016c6715162275bef5307b5b4b3f54bf))
* **finance:** migrate to pgx driver for SCRAM-SHA-256 support ([e914972](https://github.com/mutugading/goapps-backend/commit/e9149724451a53f2bbd0f538d076e9feff7a9583))
