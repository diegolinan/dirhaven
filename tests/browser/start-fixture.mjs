import {spawn} from 'node:child_process';
const child=spawn('go',['test','./internal/setupwizard','-run','^TestSetupBrowserFixture$','-v','-count=1','-timeout=9m'],{env:{...process.env,DIRHAVEN_UI_FIXTURE:'1'},stdio:'inherit'});
child.on('exit',code=>process.exitCode=code??1);
async function stop(){try{await fetch('http://127.0.0.1:4189/__fixture/stop',{method:'POST'});}catch{} }
process.on('SIGTERM',stop);process.on('SIGINT',stop);