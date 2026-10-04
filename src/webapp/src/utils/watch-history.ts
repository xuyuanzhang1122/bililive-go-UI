export interface PlayRecord {relPath:string;name:string;position:number;duration:number;savedAt:number;recording_id?:string;source_version?:string;origin?:string;}
const HISTORY_KEY='bililive_play_history';
export function loadPlayRecord(): PlayRecord | null {
  try {const value=localStorage.getItem(HISTORY_KEY);return value?JSON.parse(value):null;} catch {return null;}
}
export function savePlayRecord(relPath:string,name:string,position:number,duration:number,identity?:{recording_id:string;source_version:string}): PlayRecord | null {
  try {const record:PlayRecord={relPath,name,position,duration,savedAt:Date.now(),...identity,origin:window.location.origin};localStorage.setItem(HISTORY_KEY,JSON.stringify(record));return record;} catch {return null;}
}
export function resumePosition(record: PlayRecord | null,id:string,version:string,origin:string): number {
  if(!record || !id || !version || record.recording_id!==id || record.source_version!==version || record.origin!==origin || !Number.isFinite(record.position) || !Number.isFinite(record.duration) || record.duration<=0) return 0;
  return record.position>5 && record.position<record.duration*0.95 ? record.position : 0;
}
export function roomSizeText(room:{total_size:number;total_size_text?:string;statistics_status?:string}): string {
  if(room.statistics_status && room.statistics_status!=='verified') return '统计暂不可用';
  if(room.total_size_text) return room.total_size_text;
  if(!Number.isFinite(room.total_size) || room.total_size<0) return '统计暂不可用';
  let size=room.total_size,index=0;const units=['B','kB','MB','GB','TB','PB'];while(size>=1000 && index<units.length-1){size/=1000;index++;}
  return index===0?`${size} B`:`${size.toFixed(1)} ${units[index]}`;
}
