"use strict";
(() => {
  const byId = id => document.getElementById(id);
  const shell = document.querySelector('.app-shell');
  const title = byId('screenTitle');
  const panels = [...document.querySelectorAll('.panel')];
  const next = byId('nextStep');
  const previous = byId('previousStep');
  let screen = 1;
  let model = {scope:null,saved:null,savedLoaded:false,busy:false,sessionActive:null};
  let resolvedPreferences = false;
  let interacted = false;
  const screens = {
    1:['01 / Your Workspace','Choose Your Folders.','Start with the directories you want your MCP client to access. You can change the selection before preparing your connection.','dirhaven-symbol.webp'],
    2:['02 / Your Connection','Make The Connection.','Use the prepared scope with a client you control. Dirhaven does not start or reconfigure your connection.','setup-connection.webp'],
    3:['03 / A Real Operation','Try It In Your Client.','A prepared configuration is a starting point. Check what actually happens through your connected client.','setup-inspect.webp'],
    manage:['Local / Your Configuration','Your Workspace.','Review a saved selection, prepare a different scope or return to connection instructions.','dirhaven-symbol.webp']
  };
  function destination() {return document.querySelector('input[name="destination"]:checked').value;}
  function updateDestination() {
    const chatgpt = destination() === 'chatgpt';
    document.querySelectorAll('.destination-chatgpt').forEach(e => {e.hidden = !chatgpt;});
    document.querySelectorAll('.destination-stdio').forEach(e => {e.hidden = chatgpt;});
    byId('clientExample').open = !chatgpt;
  }
  function rootList(node, roots, removable=false) {
    node.replaceChildren();
    roots.forEach(path => {
      const li = document.createElement('li');
      const label = document.createElement('span');label.textContent = path;li.append(label);
      if(removable) {
        const button=document.createElement('button');button.type='button';button.className='remove-folder';button.textContent='Remove';button.setAttribute('aria-label','Remove folder '+path);
        button.addEventListener('click',()=>{
          const field=byId('paths');field.value=field.value.split(/\r?\n/).filter(p=>p.trim()!==path).join('\n');field.dispatchEvent(new Event('input',{bubbles:true}));
          const remaining=[...node.querySelectorAll('button')];(remaining[0] || byId('pickDirectory')).focus();
        });li.append(button);
      }
      node.append(li);
    });
  }
  function renderFolders() {rootList(byId('selectedFolderList'),[...new Set(byId('paths').value.split(/\r?\n/).map(p=>p.trim()).filter(Boolean))],true);}
  function renderState() {
    const ready=Boolean(model.scope)&&!model.busy;
    document.querySelectorAll('#stepNav [data-step]').forEach(b=>{b.disabled=Number(b.dataset.step)>1&&!ready;});
    next.disabled=screen!== 'manage'&&!ready;
    byId('overviewLink').hidden=!model.saved&&!model.scope&&screen!=='manage';
    byId('sessionExpired').hidden=model.sessionActive !== false;
    byId('savedReadNotice').hidden=!model.savedReadError;
    byId('savedReadNotice').textContent=model.savedMessage || '';
    const initialMessage=!model.savedMessage || model.savedMessage.startsWith('Saved locally:') || model.savedMessage.startsWith('No saved choices');
    byId('overviewFeedback').hidden=initialMessage;
    byId('overviewFeedback').textContent=initialMessage?'':model.savedMessage;
    byId('saveFeedback').textContent=model.savedLoaded?model.savedMessage:'';
    const roots=model.saved?.roots || [];
    rootList(byId('savedFolderList'),roots);
    byId('overviewStatus').textContent=model.saved?'Saved selection available':model.scope?'Configuration prepared':'No saved selection';
    byId('overviewDetail').textContent=model.saved?.mode==='all'?'Broad access was saved. Review it and confirm again before preparing a new command.':model.saved?roots.length+' folder'+(roots.length===1?'':'s')+' saved for review.':model.scope?'The command is prepared for this session. Saving remains optional.':'Choose folders to prepare your local command.';
    byId('manageLocalStatus').textContent=model.scope?'Prepared in this session':'Needs validation';
    byId('scopeSummary').textContent=model.scope?.mode==='all'?'Broad access, explicitly acknowledged.':model.scope?.paths.join(' · ') || '';
    byId('stepHint').textContent=screen===1?(ready?'Scope prepared. Saving is optional.':'Validate your folders to continue.'):screen===2?'Your existing connection remains unchanged.':'Check the result in your client before relying on it.';
    if(document.querySelector('input[name=mode]:checked').value==='all') byId('broaderAccess').open=true;
    renderFolders();
  }
  function showScreen(target,focus=true) {
    if(target!== 'manage')target=Number(target);
    if((target===2||target===3)&&(!model.scope||model.busy))target=1;
    screen=target;shell.dataset.screen=String(target);
    for(const panel of panels)panel.hidden=panel.id!=='panel-'+target;
    const spec=screens[target];byId('screenEyebrow').textContent=spec[0];title.textContent=spec[1];byId('screenDescription').textContent=spec[2];byId('stageArt').src='/assets/'+spec[3];
    byId('stepNav').hidden=target==='manage';byId('stepActions').hidden=target==='manage';previous.hidden=target===1||target==='manage';
    next.textContent=target===1?'Prepare connection →':target===2?'Try it in your client →':'View summary →';
    document.querySelectorAll('#stepNav [data-step]').forEach(b=>{if(Number(b.dataset.step)===target)b.setAttribute('aria-current','step');else b.removeAttribute('aria-current');});
    updateDestination();renderState();
    if(focus){title.focus({preventScroll:true});window.scrollTo({top:0,behavior:'instant'});byId('viewAnnouncement').textContent=target==='manage'?'Configuration overview.':'Step '+target+' of 3.';}
  }
  window.addEventListener('dirhaven:state',event=>{
    model=event.detail;
    if(model.savedLoaded&&!resolvedPreferences){resolvedPreferences=true;if(model.saved&&!interacted){showScreen('manage',false);return;}}
    if((screen===2||screen===3)&&!model.scope){showScreen(1);return;}
    renderState();
  });
  document.querySelectorAll('[data-step]').forEach(b=>b.addEventListener('click',()=>{interacted=true;showScreen(b.dataset.step);}));
  next.addEventListener('click',()=>{interacted=true;showScreen(screen===3?'manage':screen+1);});
  previous.addEventListener('click',()=>showScreen(screen-1));
  byId('brandHome').addEventListener('click',()=>{interacted=true;showScreen(model.saved||model.scope?'manage':1);});
  byId('overviewLink').addEventListener('click',()=>{interacted=true;showScreen('manage');});
  byId('configureFresh').addEventListener('click',()=>{interacted=true;byId('paths').value='';byId('ack').checked=false;const scoped=document.querySelector('input[name=mode][value=scoped]');scoped.checked=true;scoped.dispatchEvent(new Event('change',{bubbles:true}));showScreen(1);});
  byId('loadSaved').addEventListener('click',()=>{interacted=true;showScreen(1);});
  byId('paths').addEventListener('input',()=>{interacted=true;renderFolders();});
  byId('pickDirectory').addEventListener('click',()=>{interacted=true;});
  document.querySelectorAll('input[name="destination"]').forEach(r=>r.addEventListener('change',()=>{interacted=true;updateDestination();}));
  document.querySelectorAll('input[name="connectionRoute"]').forEach(r=>r.addEventListener('change',()=>{const existing=r.value==='existing';byId('existingRoute').hidden=!existing;byId('newRoute').hidden=existing;}));
  document.addEventListener('input',()=>{interacted=true;});
  showScreen(1,false);
  window.dispatchEvent(new Event('dirhaven:request-state'));
  shell.dataset.ready = 'true';
})();