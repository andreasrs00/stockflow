import {describe,it,expect} from 'vitest';
import {positiveInteger,validateOrder,money,relativeTime,MAX_INT} from './logic';
describe('quantity validation',()=>{
 it.each(['0','-1','1.5','','NaN','Infinity','1e2',String(MAX_INT+1)])('rejects %s',value=>expect(positiveInteger(value)).toBeNull());
 it('accepts positive integers',()=>{expect(positiveInteger('12')).toBe(12);expect(positiveInteger(' 2 ')).toBe(2)});
});
describe('order validation',()=>{
 it('requires a customer',()=>expect(validateOrder({customer:' ',items:[{sku:'A',qty:1}]})).toContain('customer'));
 it('rejects empty items',()=>expect(validateOrder({customer:'A',items:[]})).toContain('Add'));
 it('rejects duplicate trimmed SKU',()=>expect(validateOrder({customer:'A',items:[{sku:'A',qty:1},{sku:' A ',qty:1}]})).toContain('Duplicate'));
 it('rejects fractional quantities',()=>expect(validateOrder({customer:'A',items:[{sku:'A',qty:1.5}]})).toContain('positive integer'));
 it('accepts a valid order',()=>expect(validateOrder({customer:'A',items:[{sku:'A',qty:1}]})).toBeNull());
});
it('formats integer rupiah',()=>expect(money(1234567)).toBe('Rp 1.234.567'));
it('handles missing and relative movement dates',()=>{expect(relativeTime(null)).toBe('No movement');expect(relativeTime('2025-01-01T00:00:00Z',Date.parse('2025-01-01T02:00:00Z'))).toBe('2h ago')});
