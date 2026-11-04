import { useQuery } from '@tanstack/react-query';
import { api } from '@classding/api';
import type { School } from './lib';

// Teachers who registered on their own belong to no school; selects use this value for them.
export const NO_SCHOOL = 'none';

/** Loads the schools the administrator can see and names organizations by ID. */
export function useSchools() {
  const query = useQuery({ queryKey: ['admin', 'schools'], queryFn: () => api<School[]>('/admin/schools') });
  const schools = query.data ?? [];
  const names = new Map(schools.map((school) => [school.id, school.name]));
  return {
    query,
    schools,
    isSchool: (orgId?: string | null) => !!orgId && names.has(orgId),
    schoolName: (orgId?: string | null, none = '未加入学校') => (orgId && names.get(orgId)) || none,
    options: (none: string) => [
      { value: NO_SCHOOL, label: none },
      ...schools.map((school) => ({ value: school.id, label: school.name })),
    ],
  };
}
