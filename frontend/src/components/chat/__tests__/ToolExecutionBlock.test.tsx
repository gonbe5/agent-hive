import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ToolExecutionBlock } from '../ToolExecutionBlock';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => {
      const map: Record<string, string> = {
        'tools.clickToExpand': 'Click to expand',
        'tools.clickToCollapse': 'Click to collapse',
        'tools.output': 'Output',
        'tools.input': 'Input',
        'tools.truncated': '(truncated)',
        'chat.generating': 'Generating...',
      };
      return map[key] ?? key;
    },
  }),
}));

vi.mock('../../../store/chat', () => ({
  useChatStore: (selector: (s: unknown) => unknown) => selector({ toolCallStatuses: {} }),
}));

vi.mock('../../../utils/toolName', () => ({
  getToolDisplayName: (name: string) => (name === 'bash_exec' ? 'Shell' : name),
}));

describe('ToolExecutionBlock', () => {
  const baseProps = {
    id: 'tc-1',
    name: 'bash_exec',
    args: '{"command":"ls"}',
    result: 'file1\nfile2',
  };

  it('renders collapsed by default', () => {
    render(<ToolExecutionBlock {...baseProps} status="success" />);
    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('aria-expanded', 'false');
    expect(button.textContent).toBe('Click to expand');
    expect(screen.queryByText('Input')).toBeNull();
  });

  it('expands when toggle button is clicked', () => {
    render(<ToolExecutionBlock {...baseProps} status="success" />);
    const button = screen.getByRole('button');
    fireEvent.click(button);
    expect(button).toHaveAttribute('aria-expanded', 'true');
    expect(button.textContent).toBe('Click to collapse');
    expect(screen.getByText('Input')).toBeTruthy();
  });

  it('disables toggle button when running', () => {
    render(<ToolExecutionBlock {...baseProps} status="running" />);
    const button = screen.getByRole('button');
    expect(button).toBeDisabled();
  });

  it('renders danger-colored X icon on error', () => {
    const { container } = render(
      <ToolExecutionBlock {...baseProps} status="error" />
    );
    const svgs = container.querySelectorAll('svg');
    const hasDangerIcon = Array.from(svgs).some((el) =>
      (el.getAttribute('style') || '').includes('var(--danger)')
    );
    expect(hasDangerIcon).toBe(true);
  });

  it('renders input and output sections when expanded', () => {
    render(<ToolExecutionBlock {...baseProps} status="success" />);
    fireEvent.click(screen.getByRole('button'));
    expect(screen.getByText('Input')).toBeTruthy();
    expect(screen.getAllByText('Output').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText(/file1/)).toBeTruthy();
  });

  it('hides input section when args are empty object', () => {
    render(
      <ToolExecutionBlock
        id="tc-2"
        name="bash_exec"
        args="{}"
        result="done"
        status="success"
      />
    );
    fireEvent.click(screen.getByRole('button'));
    expect(screen.queryByText('Input')).toBeNull();
  });

  it('formats duration >= 1000ms in seconds', () => {
    render(<ToolExecutionBlock {...baseProps} status="success" duration={2345} />);
    expect(screen.getByText('2.3s')).toBeTruthy();
  });

  it('formats duration < 1000ms in ms', () => {
    render(<ToolExecutionBlock {...baseProps} status="success" duration={142} />);
    expect(screen.getByText('142ms')).toBeTruthy();
  });
});
