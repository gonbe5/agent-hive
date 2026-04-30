import { useId } from 'react';

export function ClawIcon({ className = '' }: { className?: string }) {
  const id = useId();
  const gradId = `claw-grad-${id}`;
  return (
    <svg className={className} viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg">
      <defs>
        <linearGradient id={gradId} x1="2" y1="10" x2="18" y2="10" gradientUnits="userSpaceOnUse">
          <stop offset="0%" stopColor="#60A5FA"/>
          <stop offset="100%" stopColor="#3B82F6"/>
        </linearGradient>
      </defs>
      <polygon points="15.42,7.5 17.58,8.75 17.58,11.25 15.42,12.5 13.25,11.25 13.25,8.75" fill={`url(#${gradId})`}/>
      <polygon points="4.58,7.5 6.75,8.75 6.75,11.25 4.58,12.5 2.42,11.25 2.42,8.75" fill={`url(#${gradId})`}/>
      <polygon points="12.71,2.81 14.88,4.06 14.88,6.56 12.71,7.81 10.54,6.56 10.54,4.06" fill={`url(#${gradId})`}/>
      <polygon points="7.29,2.81 9.46,4.06 9.46,6.56 7.29,7.81 5.13,6.56 5.13,4.06" fill={`url(#${gradId})`}/>
      <polygon points="12.71,12.19 14.88,13.44 14.88,15.94 12.71,17.19 10.54,15.94 10.54,13.44" fill={`url(#${gradId})`}/>
      <polygon points="7.29,12.19 9.46,13.44 9.46,15.94 7.29,17.19 5.13,15.94 5.13,13.44" fill={`url(#${gradId})`}/>
    </svg>
  );
}
