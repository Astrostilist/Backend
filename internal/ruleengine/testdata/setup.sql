INSERT INTO astro_rules (name, astro_condition, product_tags, priority, is_active) 
VALUES
('Венера в Тельце', '[""]'::jsonb, '["romantic", "luxury"]'::jsonb, 5, TRUE),
('Венера в Тельце', '[""]'::jsonb, '["romantic", "cheap"]'::jsonb,  3, TRUE),
('Венера в Тельце', '[""]'::jsonb, '["usual"]'::jsonb,              1, FALSE),
('Полнолуние',      '[""]'::jsonb, '["luxury"]'::jsonb,             5, TRUE),
('Новолуние',       '[""]'::jsonb, '["luxury"]'::jsonb,             5, TRUE),
('Марс в Стрельце', '[""]'::jsonb, '["luxury"]'::jsonb,             5, FALSE),
('Меркурий в Т',    '[""]'::jsonb, '["mem1"]'::jsonb,               1, TRUE),
('Меркурий в Т',    '[""]'::jsonb, '["mem_prior10"]'::jsonb,        10, TRUE);

