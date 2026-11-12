import { router } from './router';
export function sectionPath(section: string) {
  return section === 'certificates' ? '/websites/ssl' : '/' + section.replace(/^\//, '');
}
export function navigate(section: string) {
  return router.push(sectionPath(section));
}
