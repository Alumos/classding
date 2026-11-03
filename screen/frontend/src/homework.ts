import type { components } from '@classding/api/types';
export type Homework = components['schemas']['Homework'];
export function readHomeworks(input: unknown, classId: string): Homework[] | null {
  if (!Array.isArray(input) || input.length > 12) return null;
  const seen = new Set<string>();
  for (const value of input) {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
    const h = value as Partial<Homework>;
    if (
      typeof h.id !== 'string' ||
      !h.id ||
      seen.has(h.id) ||
      typeof h.class_id !== 'string' ||
      (classId && h.class_id !== classId) ||
      typeof h.subject !== 'string' ||
      !h.subject.trim() ||
      Array.from(h.subject).length > 24 ||
      typeof h.text !== 'string' ||
      !h.text.trim() ||
      Array.from(h.text).length > 300 ||
      typeof h.teacher_name !== 'string' ||
      typeof h.updated_at !== 'string' ||
      typeof h.visible !== 'boolean' ||
      typeof h.always_on_top !== 'boolean'
    )
      return null;
    seen.add(h.id);
  }
  return input as Homework[];
}
