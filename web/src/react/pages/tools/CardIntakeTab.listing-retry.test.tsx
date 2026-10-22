import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { api } from '../../../js/api';
import type { ConfirmedReturnState, Purchase } from '../../../types/campaigns';
import CardIntakeTab from './CardIntakeTab';
import CardIntakeReturnNotice from './CardIntakeReturnNotice';
import { rowCanRetryListing, rowIsListable } from './cardIntakeTypes';
import type { CertRow } from './cardIntakeTypes';
import { saveQueue } from './cardIntakeStorage';

const stamp = '2026-10-21T12:00:00Z';
const purchase: Purchase = {
 id: 'p', campaignId: 'c', certNumber: '123008718', cardName: 'Card', grader: 'PSA',
 gradeValue: 6, buyCostCents: 1000, clValueCents: 5000, receivedAt: stamp,
 psaSourcingFeeCents: 0, purchaseDate: '2026-09-01', createdAt: stamp, updatedAt: stamp,
 dhInventoryId: 42, dhCardId: 100, dhStatus: 'in_stock', dhPushStatus: 'matched',
};
function state(retryable?: boolean): ConfirmedReturnState {
 return { operation: null, expectedSaleId: null, awaitingListing: false, sale: null, purchase,
 precedingAttempt: retryable === undefined ? null : {
  id: 'a', capturedPurchaseId: 'p', dhInventoryId: 42, certNumber: purchase.certNumber,
  grader: 'PSA', kind: 'list', phase: 'intake_patch_sync', startedAt: stamp, outcome: 'open', retryable,
 }};
}
function row(s = state(true)): CertRow {
 return { certNumber: purchase.certNumber, purchaseId: 'p', status: 'existing',
  dhInventoryId: 42, dhStatus: 'in_stock', market: {clValueCents:5000,gradePriceCents:4000,lastSoldCents:4000},
  returnState: s };
}
function transport(initial: ConfirmedReturnState) {
 let durable = initial;
 const read = vi.spyOn(api,'getConfirmedReturnState').mockImplementation(async () => durable);
 const save = vi.spyOn(api,'setReviewedPrice').mockResolvedValue({success:true,reviewedAt:stamp});
 const list = vi.spyOn(api,'listPurchaseOnDH').mockImplementation(async () => {
  durable = {...state(),purchase:{...purchase,dhStatus:'listed'}};
  return {listed:1,synced:1,skipped:0,total:1};
 });
 vi.spyOn(api,'fetchWithRetry').mockRejectedValue(new Error('Unexpected API call'));
 saveQueue(new Map([[purchase.certNumber,row()]]));
 return {read,save,list,set:(s:ConfirmedReturnState)=>{durable=s;}};
}
beforeEach(() => {
 const saved = new Map<string,string>();
 vi.spyOn(localStorage,'getItem').mockImplementation(k=>saved.get(k)??null);
 vi.spyOn(localStorage,'setItem').mockImplementation((k,v)=>{saved.set(k,v);});
 vi.spyOn(localStorage,'removeItem').mockImplementation(k=>{saved.delete(k);});
});
afterEach(()=>vi.restoreAllMocks());

it.each([
 ['allowed', (r:CertRow)=>r, true],
 ['unknown marker', (r:CertRow)=>({...r,returnState:state(false)}), false],
 ['wrong mutation kind', (_r:CertRow)=>{const s=state(true);s.precedingAttempt!.kind='price';return row(s);}, false],
 ['wrong phase', (_r:CertRow)=>{const s=state(true);s.precedingAttempt!.phase='other';return row(s);}, false],
 ['wrong cert', (_r:CertRow)=>{const s=state(true);s.precedingAttempt!.certNumber='other';return row(s);}, false],
 ['settled attempt', (_r:CertRow)=>{const s=state(true);s.precedingAttempt!.outcome='succeeded';return row(s);}, false],
 ['wrong purchase', (r:CertRow)=>({...r,purchaseId:'other'}), false],
 ['wrong target', (r:CertRow)=>({...r,dhInventoryId:99}), false],
 ['busy', (r:CertRow)=>({...r,listingStatus:'listing' as const}), false],
 ['stale', (r:CertRow)=>({...r,returnStale:true}), false],
 ['read failed', (r:CertRow)=>({...r,returnStatusError:'failed'}), false],
 ['already listed', (r:CertRow)=>({...r,listingStatus:'listed' as const}), false],
 ['sale', (r:CertRow)=>({...r,status:'sold' as const}), false],
] as const)('gates listing retry: %s', (_name,change,want)=>{
 const candidate=change(row());
 expect(rowCanRetryListing(candidate)).toBe(want);
 expect(rowIsListable(candidate)).toBe(false);
});

it('restores retry from server state after remount and never saves a price', async ()=>{
 const m=transport(state(true));
 const first=render(<CardIntakeTab/>);
 await screen.findByRole('button',{name:'Retry listing'});
 first.unmount();
 render(<CardIntakeTab/>);
 await userEvent.click(await screen.findByRole('button',{name:'Retry listing'}));
 await waitFor(()=>expect(m.list).toHaveBeenCalledTimes(1));
 expect(m.save).not.toHaveBeenCalled();
 await waitFor(()=>expect(screen.queryByRole('button',{name:'Retry listing'})).not.toBeInTheDocument());
});

it('refreshes retry permission after an initial listing failure and keeps its error',async ()=>{
 const m=transport(state());
 m.list.mockImplementationOnce(async ()=>{m.set(state(true));throw new Error('provider failed');});
 render(<CardIntakeTab/>);
 await userEvent.click(await screen.findByRole('button',{name:'List on DH'}));
 await screen.findByText('provider failed');
 const retry=await screen.findByRole('button',{name:'Retry listing'});
 expect(m.save).toHaveBeenCalledTimes(1);
 await userEvent.click(retry);
 await waitFor(()=>expect(m.list).toHaveBeenCalledTimes(2));
 expect(m.save).toHaveBeenCalledTimes(1);
});

it('prevents a double submission while a retry is in flight',async ()=>{
 const m=transport(state(true));
 let finish: (()=>void) | undefined;
 m.list.mockImplementationOnce(()=>new Promise(resolve=>{finish=()=>resolve({listed:1,synced:1,skipped:0,total:1});}));
 render(<CardIntakeTab/>);
 const retry=await screen.findByRole('button',{name:'Retry listing'});
 fireEvent.click(retry);fireEvent.click(retry);
 expect(m.list).toHaveBeenCalledTimes(1);
 finish?.();
});

it('does not offer retry from stale permission when refreshing state fails',async ()=>{
 const m=transport(state(true));
 m.list.mockImplementationOnce(async ()=>{m.read.mockRejectedValue(new Error('state unavailable'));throw new Error('provider failed');});
 render(<CardIntakeTab/>);
 await userEvent.click(await screen.findByRole('button',{name:'Retry listing'}));
 await screen.findByText('provider failed');
 await waitFor(()=>expect(screen.queryByRole('button',{name:'Retry listing'})).not.toBeInTheDocument());
 expect(m.save).not.toHaveBeenCalled();
});

it.each([false,true])('shows retry progress without a stale alert (returned=%s)', returned=>{
 const s=state(true);
 if (returned) s.operation={
  id:'return-operation',purchaseId:'p',capturedPurchaseId:'p',dhInventoryId:42,
  certNumber:purchase.certNumber,grader:'PSA',expectedSaleId:null,capturedOrderId:'',
  returnedOrderId:'ext-1',state:'completed',createdAt:stamp,completedAt:stamp,
 };
 render(<CardIntakeReturnNotice row={{...row(s),listingStatus:'listing'}}/>);
 expect(screen.getByRole('status')).toHaveTextContent('Retrying the same approved listing');
 expect(screen.queryByRole('alert')).not.toBeInTheDocument();
 expect(screen.queryByText(/Investigate|Diagnose/)).not.toBeInTheDocument();
});

it('keeps an old unknown-stage marker blocked',async ()=>{
 transport(state(false));render(<CardIntakeTab/>);
 await screen.findByText(/DH mutation unresolved/);
 expect(screen.queryByRole('button',{name:'Retry listing'})).not.toBeInTheDocument();
});
