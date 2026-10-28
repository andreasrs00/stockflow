import type {OrderInput} from './types';
export const MAX_INT=2147483647;
export const money=(n:number)=>'Rp '+new Intl.NumberFormat('id-ID',{maximumFractionDigits:0}).format(n);
export function positiveInteger(value:string):number|null{if(!/^\d+$/.test(value.trim()))return null;const n=Number(value);return Number.isInteger(n)&&n>0&&n<=MAX_INT?n:null}
export function validateOrder(input:OrderInput):string|null {
 if(!input.customer.trim()||input.customer.trim().length>200)return 'Enter a customer name (up to 200 characters).';
 if(!input.items.length||input.items.length>100)return 'Add between 1 and 100 items.';
 const seen=new Set<string>();
 for(const item of input.items){const sku=item.sku.trim();if(!sku)return 'Enter a SKU for every item.';if(seen.has(sku))return `Duplicate SKU: ${sku}`;seen.add(sku);if(!Number.isInteger(item.qty)||item.qty<=0||item.qty>MAX_INT)return `Enter a positive integer quantity for ${sku}.`}
 return null;
}
export function relativeTime(iso:string|null,now=Date.now()):string{
 if(!iso)return 'No movement';const seconds=Math.max(0,Math.floor((now-new Date(iso).getTime())/1000));
 if(seconds<60)return 'Just now';if(seconds<3600)return `${Math.floor(seconds/60)}m ago`;if(seconds<86400)return `${Math.floor(seconds/3600)}h ago`;return `${Math.floor(seconds/86400)}d ago`;
}
