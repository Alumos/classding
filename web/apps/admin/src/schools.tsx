import { useQuery } from '@tanstack/react-query';
import { api } from '@classding/api';
import { SimpleSelect } from '@classding/ui/components/classding';
import { useSession, type School } from './lib';

// Teachers who registered on their own belong to no school; selects use this value for them.
export const NO_SCHOOL = 'none';
export const ALL_SCHOOLS = 'all';

/** Loads the schools the administrator can see and names organizations by ID. */
export function useSchools() {
  const query = useQuery({ queryKey: ['admin', 'schools'], queryFn: () => api<School[]>('/admin/schools') });
  const schools = query.data ?? [];
  const names = new Map(schools.map((school) => [school.id, school.name]));
  const isSchool = (orgId?: string | null) => !!orgId && names.has(orgId);
  return {
    query,
    schools,
    isSchool,
    schoolName: (orgId?: string | null, none = '未加入学校') => (orgId && names.get(orgId)) || none,
    options: (none: string) => [
      { value: NO_SCHOOL, label: none },
      ...schools.map((school) => ({ value: school.id, label: school.name })),
    ],
    /** Whether a record of orgId passes a school filter value. */
    matches: (filter: string, orgId?: string | null) =>
      filter === ALL_SCHOOLS || (filter === NO_SCHOOL ? !isSchool(orgId) : orgId === filter),
  };
}

/** Narrows a list to one school. Only the operator manages several schools, so only they see it. */
export function SchoolFilter({
  value,
  onChange,
  none = '未加入学校',
}: {
  value: string;
  onChange: (value: string) => void;
  none?: string;
}) {
  const { user } = useSession();
  const { schools, options } = useSchools();
  if (user.role !== 'super_admin' || !schools.length) return null;
  return (
    <SimpleSelect
      label="按学校筛选"
      value={value}
      onValueChange={onChange}
      options={[{ value: ALL_SCHOOLS, label: '全部学校' }, ...options(none)]}
      className="w-full sm:w-44"
    />
  );
}
