BEGIN;
INSERT INTO products(sku,name,price,is_active) VALUES
 ('ST-001','A4 Copy Paper 80gsm',55000,true),('ST-002','Blue Ballpoint Pens (12 pack)',32000,true),
 ('ST-003','Shipping Carton Medium',8500,true),('ST-004','Packing Tape 48mm',12000,true),
 ('ST-005','Bubble Wrap 10m',45000,true),('ST-006','Thermal Labels 100x150',75000,true),
 ('ST-007','Safety Gloves',28000,true),('ST-008','High Visibility Vest',65000,true),
 ('ST-009','Utility Knife',18000,true),('ST-010','Permanent Marker',9000,true),
 ('ST-011','Archive Box',22000,false),('ST-012','Cable Ties (100 pack)',24000,true);
INSERT INTO stock_movements(product_id,type,qty,note)
 SELECT id,'in',CASE WHEN id IN (5,9,11) THEN 8 ELSE 60 END,'Opening stock' FROM products;
UPDATE products p SET stock=m.qty FROM stock_movements m WHERE m.product_id=p.id;
INSERT INTO sales_orders(customer,total) VALUES ('Nusantara Office',142000),('Surya Logistics',58000);
INSERT INTO sales_order_items(order_id,sku,qty,unit_price) VALUES
 (1,'ST-001',2,55000),(1,'ST-002',1,32000),(2,'ST-003',4,8500),(2,'ST-004',2,12000);
INSERT INTO stock_movements(product_id,type,qty,note,order_id)
 SELECT p.id,'out',i.qty,'Seed order',i.order_id FROM sales_order_items i JOIN products p ON p.sku=i.sku;
UPDATE products p SET stock=p.stock-q.qty FROM (SELECT sku,sum(qty)::int qty FROM sales_order_items GROUP BY sku) q WHERE p.sku=q.sku;
COMMIT;
