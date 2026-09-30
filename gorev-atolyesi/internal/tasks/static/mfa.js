'use strict';
const mq=s=>document.querySelector(s);
let securityUser=null,securityApproval=false,mfaMode='verify',mfaMandatory=false,mfaBusy=false,recoveryDisplay=[];
function clearMFA(){for(const id of ['#mfa-password','#mfa-code','#mfa-secret'])mq(id).value='';mq('#mfa-recovery-list').replaceChildren();recoveryDisplay=[];mq('#mfa-error').textContent='';}
function openMFA(mode,mandatory=false){
 clearMFA();mfaMode=mode;mfaMandatory=mandatory;
 for(const d of document.querySelectorAll('dialog[open]'))d.close();
 mq('#mfa-form').hidden=false;mq('#mfa-recovery-box').hidden=true;mq('#mfa-setup-key').hidden=true;
 mq('#mfa-password-label').hidden=mode==='verify';mq('#mfa-password').required=mode!=='verify';
 mq('#mfa-code-label').hidden=mode==='setup';mq('#mfa-code').required=mode!=='setup';
 mq('#mfa-code').maxLength=35;mq('#mfa-code').removeAttribute('pattern');mq('#mfa-code').inputMode='text';mq('#mfa-code-label').firstChild.textContent='Uygulama kodu veya kurtarma kodu';
 mq('#mfa-close').hidden=mandatory;mq('#mfa-title').textContent=mode==='setup'?'İki adımlı doğrulamayı kur':mode==='recovery'?'Kurtarma kodlarını yenile':'Girişini doğrula';
 mq('#mfa-copy').textContent=mode==='setup'?'Yönetici hesaplarında zorunludur. Kimlik doğrulama uygulaman her 30 saniyede bir yeni kod üretir.':mode==='recovery'?'Parolanı ve kullanılmamış uygulama veya kurtarma kodunu gir. Eski kurtarma kodları ve diğer oturumların iptal edilecek.':'Kimlik doğrulama uygulamandaki yeni kodu gir. Telefonuna ulaşamıyorsan tek kullanımlık kurtarma kodlarından birini kullan.';
 mq('#mfa-submit').textContent=mode==='setup'?'Kurulumu başlat':mode==='recovery'?'Yeni kurtarma kodları oluştur':'Doğrula';
 mq('#mfa-dialog').showModal();mq(mode==='verify'?'#mfa-code':'#mfa-password').focus();
}
window.atolyeMFA={
 clear(){clearMFA();securityUser=null;mq('#security-open').hidden=true;},
 update(user,approval){securityUser=user;securityApproval=approval;mq('#security-open').hidden=!(approval&&user?.email_verified&&user.status!=='suspended');
  const pending=mq('#pending-security');if(pending)pending.hidden=!(approval&&user?.email_verified&&user.status!=='suspended');
  return Boolean(approval&&user?.email_verified&&((user.mfa_enabled&&!user.mfa_verified_at)||(user.role==='superadmin'&&!user.mfa_enabled)));
 },
 challenge(){if(securityUser)openMFA(securityUser.mfa_enabled?'verify':'setup',true);},
 manage(){if(securityUser)openMFA(securityUser.mfa_enabled?'recovery':'setup');}
};
mq('#security-open').addEventListener('click',()=>window.atolyeMFA.manage());
const pendingSecurity=document.createElement('button');pendingSecurity.id='pending-security';pendingSecurity.className='quiet';pendingSecurity.textContent='İki adımlı doğrulamayı kur / yönet';pendingSecurity.hidden=true;pendingSecurity.addEventListener('click',()=>window.atolyeMFA.manage());mq('#pending-dialog').append(pendingSecurity);
mq('#mfa-close').addEventListener('click',()=>{mq('#mfa-dialog').close();clearMFA();boot();});
mq('#mfa-dialog').addEventListener('cancel',e=>{if(mfaBusy||mfaMandatory||!mq('#mfa-recovery-box').hidden)e.preventDefault();else{clearMFA();setTimeout(()=>boot(),0);}});
mq('#mfa-logout').addEventListener('click',()=>{clearMFA();mq('#logout').click();});
mq('#mfa-copy-key').addEventListener('click',async()=>{try{await navigator.clipboard.writeText(mq('#mfa-secret').value);mq('#mfa-error').textContent='Anahtar kopyalandı.';}catch{mq('#mfa-secret').select();mq('#mfa-error').textContent='Seçili anahtarı kopyalayabilirsin.';}});
mq('#mfa-form').addEventListener('submit',async e=>{
 e.preventDefault();mfaBusy=true;mq('#mfa-submit').disabled=true;mq('#mfa-close').disabled=true;mq('#mfa-logout').disabled=true;mq('#mfa-error').textContent='';
 try{
  const result=await request('/api/mfa/'+mfaMode,{method:'POST',body:JSON.stringify({password:mq('#mfa-password').value,code:mq('#mfa-code').value.trim()})});
  mq('#mfa-password').value='';mq('#mfa-code').value='';
  if(mfaMode==='setup'){
   mfaMode='confirm';mq('#mfa-secret').value=result.secret;mq('#mfa-setup-key').hidden=false;mq('#mfa-password-label').hidden=true;mq('#mfa-password').required=false;mq('#mfa-code-label').hidden=false;mq('#mfa-code').required=true;mq('#mfa-submit').textContent='İki adımlı doğrulamayı etkinleştir';mq('#mfa-code').focus();
   mq('#mfa-code-label').firstChild.textContent='Uygulamadaki 6 haneli kod';mq('#mfa-code').maxLength=6;mq('#mfa-code').pattern='[0-9]{6}';mq('#mfa-code').inputMode='numeric';
  }else if(result.recovery_codes?.length){
   mq('#mfa-secret').value='';recoveryDisplay=result.recovery_codes;mq('#mfa-form').hidden=true;mq('#mfa-recovery-box').hidden=false;mq('#mfa-close').hidden=true;mq('#mfa-saved').checked=false;mq('#mfa-done').disabled=true;
   mq('#mfa-title').textContent='Kurtarma kodlarını sakla';mq('#mfa-copy').textContent='Telefonunu kaybetsen de hesabına erişebilmen için.';
   mq('#mfa-recovery-list').replaceChildren(...recoveryDisplay.map(code=>{const li=document.createElement('li');li.textContent=code;return li;}));
  }else{mq('#mfa-dialog').close();clearMFA();await boot();}
 }catch(e){mq('#mfa-error').textContent=e.message;}finally{mfaBusy=false;mq('#mfa-submit').disabled=false;mq('#mfa-close').disabled=false;mq('#mfa-logout').disabled=false;}
});
mq('#mfa-saved').addEventListener('change',()=>{mq('#mfa-done').disabled=!mq('#mfa-saved').checked;});
mq('#mfa-done').addEventListener('click',()=>{mq('#mfa-dialog').close();clearMFA();boot();});
mq('#mfa-download').addEventListener('click',()=>{if(!recoveryDisplay.length)return;const blob=new Blob(['Görev Atölyesi — tek kullanımlık kurtarma kodları\n\n'+recoveryDisplay.join('\n')],{type:'text/plain;charset=utf-8'});const url=URL.createObjectURL(blob),a=document.createElement('a');a.href=url;a.download='atolye-kurtarma-kodlari.txt';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);});
