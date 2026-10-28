export interface Product { id:number; sku:string; name:string; price:number; stock:number; is_active:boolean; created_at:string; last_movement_at:string|null }
export interface ProductInput { sku:string; name:string; price:number; is_active:boolean }
export interface Movement { id:number; product_id:number; type:'in'|'out'; qty:number; note:string; order_id:number|null; created_at:string }
export interface OrderItem { sku:string; qty:number; unit_price:number }
export interface Order { id:number; customer:string; status:'CREATED'|'CANCELLED'; total:number; created_at:string; cancelled_at:string|null; items:OrderItem[] }
export interface OrderInput { customer:string; items:{sku:string;qty:number}[] }
export interface Page<T> { data:T[];page:number;limit:number;total:number }
export interface Summary {total_products:number;active_products:number;total_stock_value:number;low_stock_count:number}
export interface ErrorDetail {code:string;message:string;sku?:string;available?:number}
export type Filter='all'|'active'|'inactive'|'low';
