ALTER TABLE cst_product_cost
  ADD CONSTRAINT fk_cpc_job FOREIGN KEY (cpc_job_id) REFERENCES cal_job(cj_job_id);
