import type {Product,ProductInput,Page,Filter,Summary,Order,OrderInput,Movement,ErrorDetail} from './types';
export class ApiError extends Error {
 constructor(public status:number,public detail:ErrorDetail){super(detail.message);this.name='ApiError'}
}
function isRecord(value:unknown):value is Record<string,unknown>{return typeof value==='object'&&value!==null}
function parseError(value:unknown):ErrorDetail{
 if(isRecord(value)&&isRecord(value.error)){
  const e=value.error;
  return {code:typeof e.code==='string'?e.code:'API_ERROR',message:typeof e.message==='string'?e.message:'Request failed',sku:typeof e.sku==='string'?e.sku:undefined,available:typeof e.available==='number'?e.available:undefined};
 }
 return {code:'API_ERROR',message:'The server returned an unexpected response'};
}
async function request<T>(path:string,options:RequestInit={}):Promise<T>{
 let response:Response;
 try {response=await fetch(`/api${path}`,{...options,headers:{'Content-Type':'application/json',...options.headers}})}
 catch(error){if(error instanceof DOMException&&error.name==='AbortError')throw error;throw new ApiError(0,{code:'NETWORK_ERROR',message:'Cannot reach the server. Check your connection and retry.'})}
 if(response.status===204)return undefined as T;
 const body:unknown=await response.json().catch(()=>null);
 if(!response.ok)throw new ApiError(response.status,parseError(body));
 if(body===null)throw new ApiError(response.status,{code:'INVALID_RESPONSE',message:'The server returned invalid JSON'});
 return body as T;
}
const body=(value:unknown)=>JSON.stringify(value);
export const api={
 products:(search:string,page:number,filter:Filter,signal?:AbortSignal)=>request<Page<Product>>(`/products?${new URLSearchParams({search,page:String(page),limit:'10',filter})}`,{signal}),
 summary:(signal?:AbortSignal)=>request<Summary>('/summary',{signal}),
 createProduct:(p:ProductInput)=>request<Product>('/products',{method:'POST',body:body(p)}),
 updateProduct:(id:number,p:ProductInput)=>request<Product>(`/products/${id}`,{method:'PUT',body:body(p)}),
 deleteProduct:(id:number)=>request<void>(`/products/${id}`,{method:'DELETE'}),
 stock:(id:number,input:{type:'in'|'out';qty:number;note:string})=>request<Product>(`/products/${id}/stock`,{method:'POST',body:body(input)}),
 movements:(id:number)=>request<{data:Movement[]}>(`/products/${id}/movements`),
 orders:(page:number,signal?:AbortSignal)=>request<{data:Order[]}>(`/orders?page=${page}&limit=10`,{signal}),
 order:(id:number)=>request<Order>(`/orders/${id}`),
 createOrder:(input:OrderInput)=>request<Order>('/orders',{method:'POST',body:body(input)}),
 cancelOrder:(id:number)=>request<Order>(`/orders/${id}/cancel`,{method:'POST'}),
};
