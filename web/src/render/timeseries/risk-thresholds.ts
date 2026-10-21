import { createTheme, type Field } from "@grafana/data";
import type uPlot from "uplot";

export const riskTheme = createTheme({ colors: { mode: "dark" } });
const colors = new Map<string,string>();
let colorProbe:CanvasRenderingContext2D|null|undefined;
function resolveColor(name:string):string {
  const cached=colors.get(name);if(cached) return cached;
  let resolved="#85aaff";
  try {
    const candidate=riskTheme.visualization.getColorByName(name);
    if(colorProbe===undefined && typeof document!=="undefined") colorProbe=document.createElement("canvas").getContext("2d");
    if(colorProbe) {
      colorProbe.fillStyle="#010203";colorProbe.fillStyle=candidate;
      const first=colorProbe.fillStyle;
      colorProbe.fillStyle="#040506";colorProbe.fillStyle=candidate;
      if(first===colorProbe.fillStyle) resolved=String(first);
    } else if(/^#(?:[\da-f]{3,4}|[\da-f]{6}|[\da-f]{8})$/i.test(candidate)) resolved=candidate;
  } catch { /* Invalid imported colors must not break CanvasGradient or rendering. */ }
  if(colors.size>=256) colors.clear();colors.set(name,resolved);return resolved;
}
export type RiskStep = { value:number;color:string };

export function riskSteps(field:Field):RiskStep[] {
  const thresholds=field.config.thresholds;
  if (!thresholds || field.config.color?.mode!=="thresholds") return [];
  const percent=thresholds.mode==="percentage", min=Number(field.config.min),max=Number(field.config.max);
  if (percent && (!Number.isFinite(min)||!Number.isFinite(max)||max<=min)) return [];
  return thresholds.steps.filter(step=>step.value==null||Number.isFinite(step.value)).map(step=>({
    value:step.value==null?-Infinity:percent?min+Number(step.value)/100*(max-min):Number(step.value),
    color:resolveColor(step.color),
  })).sort((a,b)=>a.value-b.value);
}

export function riskColor(field:Field,value:unknown):string|undefined {
  if (value==null || !Number.isFinite(Number(value))) return undefined;
  return riskSteps(field).filter(step=>Number(value)>=step.value).at(-1)?.color;
}

// Color is a function of the historical value (vertical position), rather than
// repainting a past severe spike green when the latest observation is safe.
export function riskStroke(chart:uPlot,field:Field,axis:string,fallback:string):CanvasRenderingContext2D["strokeStyle"] {
  const steps=riskSteps(field), {top,height}=chart.bbox;
  if (steps.length<2 || height<=0) return fallback;
  const gradient=chart.ctx.createLinearGradient(0,top,0,top+height);
  gradient.addColorStop(0,riskColor(field,chart.posToVal(top,axis,true))||fallback);
  for (const step of [...steps].reverse()) {
    if (!Number.isFinite(step.value)) continue;
    const offset=(chart.valToPos(step.value,axis,true)-top)/height;
    if (!Number.isFinite(offset) || offset<=0 || offset>=1) continue;
    gradient.addColorStop(offset,step.color);
    gradient.addColorStop(offset,riskColor(field,step.value-Math.max(1,Math.abs(step.value))*1e-9)||fallback);
  }
  gradient.addColorStop(1,riskColor(field,chart.posToVal(top+height,axis,true))||fallback);
  return gradient;
}

export function drawRiskBands(chart:uPlot,field:Field,axis:string,lines=false) {
  const steps=riskSteps(field), {top,left,width,height}=chart.bbox,ctx=chart.ctx;
  if (!steps.length || height<=0) return;
  ctx.save();ctx.beginPath();ctx.rect(left,top,width,height);ctx.clip();
  if (lines) {
    ctx.setLineDash([5,4]);ctx.lineWidth=1;
    for (const step of steps) {
      if (!Number.isFinite(step.value)) continue;
      const y=chart.valToPos(step.value,axis,true);
      if(!Number.isFinite(y)) continue;
      ctx.strokeStyle=step.color;ctx.beginPath();ctx.moveTo(left,y);ctx.lineTo(left+width,y);ctx.stroke();
    }
  } else {
    ctx.globalAlpha=0.065;
    for (let i=0;i<steps.length;i++) {
      const bottom=Number.isFinite(steps[i].value)?chart.valToPos(steps[i].value,axis,true):top+height;
      const upper=i+1<steps.length?chart.valToPos(steps[i+1].value,axis,true):top;
      const y=Math.max(top,Math.min(top+height,upper)),end=Math.max(top,Math.min(top+height,bottom));
      if (end>y) {ctx.fillStyle=steps[i].color;ctx.fillRect(left,y,width,end-y);}
    }
  }
  ctx.restore();
}
