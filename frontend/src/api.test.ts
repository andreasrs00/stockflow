import {afterEach,describe,it,expect,vi} from 'vitest';
import {api,ApiError} from './api';
afterEach(()=>vi.unstubAllGlobals());
describe('API error propagation',()=>{
 it('retains SKU and availability in a 409',async()=>{
  vi.stubGlobal('fetch',vi.fn().mockResolvedValue(new Response(JSON.stringify({error:{code:'INSUFFICIENT_STOCK',message:'Not enough stock',sku:'A',available:2}}),{status:409})));
  await expect(api.createOrder({customer:'Test',items:[{sku:'A',qty:3}]})).rejects.toMatchObject({status:409,detail:{code:'INSUFFICIENT_STOCK',sku:'A',available:2}});
 });
 it('handles transport failures',async()=>{
  vi.stubGlobal('fetch',vi.fn().mockRejectedValue(new TypeError('Failed to fetch')));
  await expect(api.summary()).rejects.toMatchObject({status:0,detail:{code:'NETWORK_ERROR'}});
 });
 it('handles an empty successful deletion',async()=>{
  vi.stubGlobal('fetch',vi.fn().mockResolvedValue(new Response(null,{status:204})));
  await expect(api.deleteProduct(1)).resolves.toBeUndefined();
 });
 it('handles a non-JSON server error',async()=>{
  vi.stubGlobal('fetch',vi.fn().mockResolvedValue(new Response('Bad gateway',{status:502})));
  await expect(api.summary()).rejects.toBeInstanceOf(ApiError);
 });
});
