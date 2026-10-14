# Payroll — peta permission (spec 07 → IAM, D-05)

Format IAM: `payroll.<modul>.<entitas>.<aksi>` (4 segmen, huruf kecil, tanpa underscore).
Seed: `services/iam/migrations/postgres/000094_seed_payroll_menu_and_permissions.up.sql`.
Aksi `commit` dan `close` ditambahkan ke whitelist `chk_permission_action` di migrasi yang sama.
Kolom role = pemberian default menurut spec 07 (belum di-seed ke role kecuali SUPER_ADMIN, lihat D-12).
O=Officer, S=Supervisor, F=Finance, A=Admin.

| Spec 07 | Kode IAM | Role |
|---|---|---|
| (menu) | `payroll.module.root.view` | semua pengguna payroll |
| `master.read` | `payroll.master.employee.view` | O S F A |
| `master.write` | `payroll.master.employee.create` / `.update` | O S A |
| `employee.gl_write` | `payroll.master.orionaccount.update` | (13§4, belum ada di tabel 07) |
| `rule.write` | `payroll.rule.set.update` | A |
| aktivasi rule set (4-eyes) | `payroll.rule.set.approve` | S |
| `addition.write` | `payroll.input.addition.create` / `.update` | O S |
| `attendance.sync` | `payroll.attendance.data.sync` | O S |
| `loan.write` | `payroll.loan.request.create` / `.update` | O S |
| `loan.approve` | `payroll.loan.request.approve` | S |
| `loan.approve_exception` | `payroll.loan.exception.approve` | S (HR Manager) |
| `loan.inbound` | `payroll.loan.inbound.execute` | service token HRIS |
| `loan.disburse` | `payroll.loan.disbursement.create` | F |
| `loan.product` | `payroll.loan.product.update` / `.approve` | S F A |
| `thr.write` | `payroll.thr.event.create` / `.update`, `payroll.thr.run.execute` | O S |
| `thr.approve` | `payroll.thr.run.approve` | S |
| `thr.close` | `payroll.thr.run.close` | S |
| `thr.scheme` | `payroll.thr.scheme.update` / `.approve` | S F A |
| `slip.send` | `payroll.slip.mail.send` | O S |
| `slip.download` | `payroll.slip.file.export` (+ `payroll.salary.amount.view`) | O S F |
| `mail.admin` | `payroll.mail.config.update` / `.approve` | S(approve) A |
| `run.dry` | `payroll.run.dryrun.execute` | O S F |
| `run.commit` | `payroll.run.salary.commit` | O S |
| `period.approve` | `payroll.period.salary.approve` | S |
| `period.close` | `payroll.period.salary.close` | S |
| `period.reopen` | `payroll.period.salary.reopen` | S |
| `report.read` | `payroll.report.data.view` | O S F |
| `salary.view` | `payroll.salary.amount.view` | O S F (admin TIDAK) |
| `audit.read` | `payroll.audit.log.view` | S A |
| `formula.write` / `.approve` | `payroll.formula.rule.update` / `.approve` | A / S |
| `subsidy.write` / `.approve` | `payroll.subsidy.rule.update` / `.approve` | S F / F S |
| `fx.write` | `payroll.fx.rate.update` | F |
| `gl.setup` | `payroll.gl.setup.update` | A F |
| `gl.export` | `payroll.gl.export.execute` | F |

Catatan:
- 4-eyes (pembuat ≠ approver) dicek di API dan constraint DB, bukan oleh IAM.
- Menu: HR > Payroll (`HR_PAYROLL`, `/hr/payroll`), dikunci `payroll.module.root.view`.
- SUPER_ADMIN menerima semua kecuali `payroll.salary.amount.view` (spec 07: admin tidak melihat gaji).
