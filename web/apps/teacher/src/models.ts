import type {components} from '@classding/api';
export type User=components['schemas']['User'];
export type Class=components['schemas']['Class'];
export type Device=components['schemas']['Device'];
export type Member=components['schemas']['Member'];
export type Message=components['schemas']['Message'];
export type Phrase=components['schemas']['Phrase'];
export type AuthResponse=components['schemas']['AuthResponse'];
export type BindCode=components['schemas']['BindCode'];

export interface Homework {
  id:string;
  class_id:string;
  subject:string;
  text:string;
  teacher_id:string;
  teacher_name:string;
  visible:boolean;
  always_on_top:boolean;
  created_at:string;
  updated_at:string;
}
