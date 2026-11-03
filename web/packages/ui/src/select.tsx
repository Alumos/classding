import * as React from 'react';
import {Select as SelectPrimitive} from '@base-ui/react/select';
import {Check,ChevronDown} from 'lucide-react';

export interface SelectOption {value:string;label:string;icon?:React.ReactNode}
export function Select({value,onValueChange,options,id,label,disabled,className=''}:{value:string;onValueChange:(value:string)=>void;options:SelectOption[];id?:string;label:string;disabled?:boolean;className?:string}){
  return <SelectPrimitive.Root value={value} onValueChange={next=>{if(next!==null)onValueChange(next);}} items={options} disabled={disabled}>
    <SelectPrimitive.Trigger id={id} aria-label={label} className={`cd-select-trigger ${className}`}>
      <SelectPrimitive.Value className="cd-select-value">{()=>{
        const selected=options.find(option=>option.value===value);
        return <>{selected?.icon}<span>{selected?.label??'请选择'}</span></>;
      }}</SelectPrimitive.Value>
      <SelectPrimitive.Icon className="cd-select-chevron"><ChevronDown size={17}/></SelectPrimitive.Icon>
    </SelectPrimitive.Trigger>
    <SelectPrimitive.Portal>
      <SelectPrimitive.Positioner className="cd-select-positioner" sideOffset={6} align="start" alignItemWithTrigger={false}>
        <SelectPrimitive.Popup className="cd-select-popup">
          <SelectPrimitive.List className="cd-select-list">{options.map(option=><SelectPrimitive.Item key={option.value} value={option.value} className="cd-select-item">
            <span className="cd-select-option-icon">{option.icon}</span>
            <SelectPrimitive.ItemText>{option.label}</SelectPrimitive.ItemText>
            <SelectPrimitive.ItemIndicator className="cd-select-indicator"><Check size={17}/></SelectPrimitive.ItemIndicator>
          </SelectPrimitive.Item>)}</SelectPrimitive.List>
        </SelectPrimitive.Popup>
      </SelectPrimitive.Positioner>
    </SelectPrimitive.Portal>
  </SelectPrimitive.Root>;
}
