import {readFile} from 'node:fs/promises';
import {after, before, beforeEach, test} from 'node:test';
import assert from 'node:assert/strict';
import {initializeTestEnvironment, assertFails, assertSucceeds} from '@firebase/rules-unit-testing';
import {ref, get, set, onValue} from 'firebase/database';

let env;
before(async () => {
 env=await initializeTestEnvironment({projectId:'demo-hostelhive',database:{host:'127.0.0.1',port:19000,rules:await readFile(new URL('database.rules.json',import.meta.url),'utf8')}});
});
after(async()=>{await env?.cleanup();});
beforeEach(async()=>{
 await env.clearDatabase();
 await env.withSecurityRulesDisabled(async context=>{
  await set(ref(context.database(),'hostelhive'),{
   attendance:{version:1,summary:{total:2,in:1,out:0,unknown:1}},
   staffAccess:{admin:{version:1,expires_at:Date.now()+300000},warden:{version:1,expires_at:Date.now()+300000},expired:{version:1,expires_at:Date.now()-1000}}
  });
 });
});

test('only a current backend-owned staff grant allows projection reads',async()=>{
 for(const uid of ['admin','warden']) await assertSucceeds(get(ref(env.authenticatedContext(uid).database(),'hostelhive/attendance')));
 for(const uid of ['student','security','expired']) await assertFails(get(ref(env.authenticatedContext(uid,{role:'admin'}).database(),'hostelhive/attendance')));
 await assertFails(get(ref(env.unauthenticatedContext().database(),'hostelhive/attendance')));
});

test('clients cannot write projections or grants, or read broader paths',async()=>{
 const db=env.authenticatedContext('admin').database();
 for(const path of ['hostelhive/attendance','hostelhive/attendance/summary','hostelhive/staffAccess/admin','hostelhive/staffAccess/student','unrelated']) await assertFails(set(ref(db,path),{expires_at:Date.now()+300000}));
 for(const path of ['/','hostelhive','hostelhive/staffAccess','hostelhive/staffAccess/admin','unrelated']) await assertFails(get(ref(db,path)));
});

function listener(db) {
 let seenResolve,cancelResolve;
 const seen=new Promise(resolve=>{seenResolve=resolve});
 const cancelled=new Promise(resolve=>{cancelResolve=resolve});
 const unsubscribe=onValue(ref(db,'hostelhive/attendance'),()=>seenResolve(),error=>cancelResolve(error));
 return {seen,cancelled,unsubscribe};
}
async function within(promise) {
 let timer;
 try{return await Promise.race([promise,new Promise((_,reject)=>{timer=setTimeout(()=>reject(new Error('listener did not cancel')),5000)})]);}
 finally{clearTimeout(timer);}
}

test('revocation cancels an already established listener',async()=>{
 const db=env.authenticatedContext('warden').database();
 const subscription=listener(db);
 try{
  await within(subscription.seen);
  await env.withSecurityRulesDisabled(async context=>{await set(ref(context.database(),'hostelhive/staffAccess/warden'),{version:2,expires_at:0});});
  assert.match((await within(subscription.cancelled)).code,/PERMISSION_DENIED/i);
  await assertFails(get(ref(db,'hostelhive/attendance')));
 } finally{subscription.unsubscribe();}
});

test('expiry denies fresh reads; an explicit worker tombstone cancels the listener',async()=>{
 await env.withSecurityRulesDisabled(async context=>{await set(ref(context.database(),'hostelhive/staffAccess/warden'),{version:2,expires_at:Date.now()+1500});});
 const db=env.authenticatedContext('warden').database();
 const subscription=listener(db);
 try{
  await within(subscription.seen);
  await new Promise(resolve=>setTimeout(resolve,1700));
  const fresh=env.authenticatedContext('warden',{session:'fresh'}).database();
  await assertFails(get(ref(fresh,'hostelhive/attendance')));
  // Clock expiry is not a cancellation scheduler. Model the backend expiry worker.
  await env.withSecurityRulesDisabled(async context=>{await set(ref(context.database(),'hostelhive/staffAccess/warden'),{version:3,expires_at:0});});
  assert.match((await within(subscription.cancelled)).code,/PERMISSION_DENIED/i);
  await assertFails(get(ref(db,'hostelhive/attendance')));
 } finally{subscription.unsubscribe();}
});
