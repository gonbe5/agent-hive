import { useState, useRef, useCallback, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Streamdown } from 'streamdown';
import {
  BookOpen,
  Rocket,
  Layout,
  MessageSquare,
  Wrench,
  Zap,
  Settings2,
  Share2,
  Code2,
  CheckCircle2,
  Circle,
  ChevronDown,
  ChevronUp,
  ChevronLeft,
  ChevronRight,
  Lightbulb,
  RotateCcw,
  ArrowLeft,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

// ---- 数据结构定义 ----

interface GuideStepDef {
  key: string;
}

interface GuideSectionDef {
  key: string;
  icon: LucideIcon;
  steps: GuideStepDef[];
}

const GUIDE_SECTIONS: GuideSectionDef[] = [
  {
    key: 'quickStart',
    icon: Rocket,
    steps: [{ key: 'prerequisites' }, { key: 'buildAndRun' }, { key: 'verify' }],
  },
  {
    key: 'webuiBasics',
    icon: Layout,
    steps: [{ key: 'layout' }, { key: 'themeAndLang' }, { key: 'dashboard' }],
  },
  {
    key: 'sessionManagement',
    icon: MessageSquare,
    steps: [{ key: 'createSession' }, { key: 'sessionList' }, { key: 'deleteSession' }, { key: 'forkRevert' }],
  },
  {
    key: 'chatWithAgent',
    icon: Wrench,
    steps: [{ key: 'sendMessage' }, { key: 'responseStates' }, { key: 'messageActions' }, { key: 'canvas' }],
  },
  {
    key: 'toolsAndSkills',
    icon: Zap,
    steps: [{ key: 'allTools' }, { key: 'viewAgents' }, { key: 'viewSkills' }, { key: 'hitl' }, { key: 'parallelTasks' }],
  },
  {
    key: 'configuration',
    icon: Settings2,
    steps: [{ key: 'configFile' }, { key: 'llmProvider' }, { key: 'profiles' }, { key: 'extensionsPlugins' }],
  },
  {
    key: 'imChannels',
    icon: Share2,
    steps: [{ key: 'overview' }, { key: 'channelConfig' }, { key: 'webhook' }],
  },
  {
    key: 'apiReference',
    icon: Code2,
    steps: [{ key: 'healthAndSession' }, { key: 'chatApi' }, { key: 'websocket' }],
  },
];

// 计算总步骤数
const TOTAL_STEPS = GUIDE_SECTIONS.reduce((sum, s) => sum + s.steps.length, 0);

// 扁平化步骤列表，用于上一步/下一步导航
interface FlatStep {
  sectionKey: string;
  stepKey: string;
  index: number; // 全局序号（0-based）
}

const FLAT_STEPS: FlatStep[] = GUIDE_SECTIONS.flatMap((section) =>
  section.steps.map((step) => ({
    sectionKey: section.key,
    stepKey: step.key,
    index: 0, // 下面赋值
  }))
).map((s, i) => ({ ...s, index: i }));

// 生成步骤 ID
function stepId(sectionKey: string, stepKey: string) {
  return `${sectionKey}.${stepKey}`;
}

// ---- 进度条组件 ----

function GuideProgressBar({
  done,
  total,
  onReset,
}: {
  done: number;
  total: number;
  onReset: () => void;
}) {
  const { t } = useTranslation();
  const pct = total > 0 ? (done / total) * 100 : 0;

  return (
    <div className="flex items-center gap-4 mb-6 bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl shadow-sm p-5">
      <div className="flex-1">
        <div className="flex items-center justify-between mb-2">
          <span className="text-sm font-medium text-[var(--text-primary)]">
            {t('guide.progressLabel')}
          </span>
          <span className="text-sm text-[var(--text-secondary)]">
            {t('guide.progress', { done, total })}
          </span>
        </div>
        <div className="h-2 bg-[var(--bg-secondary)] rounded-full overflow-hidden">
          <div
            className="h-full bg-gradient-to-r from-[var(--accent-500)] to-[var(--accent-600)] rounded-full transition-all duration-500"
            style={{ width: `${pct}%` }}
          />
        </div>
      </div>
      <button
        onClick={onReset}
        className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-secondary)] transition-colors"
        title={t('guide.resetProgress')}
      >
        <RotateCcw className="w-3.5 h-3.5" />
        <span className="hidden sm:inline">{t('guide.resetProgress')}</span>
      </button>
    </div>
  );
}

// ---- 目录导航组件 ----

function GuideTOC({
  sections,
  completed,
  activeSectionKey,
  onNavigate,
}: {
  sections: GuideSectionDef[];
  completed: Set<string>;
  activeSectionKey: string;
  onNavigate: (sectionKey: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  // 计算每个 section 的完成数
  const sectionProgress = (section: GuideSectionDef) => {
    const done = section.steps.filter((step) => completed.has(stepId(section.key, step.key))).length;
    return { done, total: section.steps.length };
  };

  const handleClick = (key: string) => {
    onNavigate(key);
    setOpen(false); // 移动端点击后折叠
  };

  const renderItem = (section: GuideSectionDef) => {
    const Icon = section.icon;
    const { done, total } = sectionProgress(section);
    const allDone = done === total;
    const isActive = section.key === activeSectionKey;
    return (
      <button
        key={section.key}
        onClick={() => handleClick(section.key)}
        className={`w-full flex items-center gap-2.5 px-3 py-2 text-sm rounded-lg transition-colors text-left ${
          isActive
            ? 'bg-[var(--accent-50)] dark:bg-[var(--accent-light)] text-[var(--accent-600)] dark:text-[var(--accent-300)] font-medium'
            : allDone
              ? 'text-emerald-600 dark:text-emerald-400'
              : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-secondary)]'
        }`}
      >
        <Icon className="w-4 h-4 shrink-0" />
        <span className="flex-1 truncate">{t(`guide.${section.key}.title`)}</span>
        <span className="text-xs tabular-nums">
          {done}/{total}
        </span>
      </button>
    );
  };

  return (
    <>
      {/* 移动端：可折叠面板 */}
      <div className="lg:hidden mb-4">
        <button
          onClick={() => setOpen(!open)}
          className="w-full flex items-center justify-between px-5 py-4 bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl shadow-sm text-sm font-medium text-[var(--text-primary)]"
        >
          <span>{t('guide.tableOfContents')}</span>
          {open ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
        </button>
        {open && (
          <div className="mt-2 p-3 bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl shadow-sm">
            <nav className="space-y-1">
              {sections.map(renderItem)}
            </nav>
          </div>
        )}
      </div>

      {/* 桌面端：sticky 侧栏 */}
      <div className="hidden lg:block w-64 shrink-0">
        <div className="sticky top-0 p-3 bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl shadow-sm">
          <h3 className="px-3 py-2 text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider">
            {t('guide.tableOfContents')}
          </h3>
          <nav className="space-y-1">
            {sections.map(renderItem)}
          </nav>
        </div>
      </div>
    </>
  );
}

// ---- 步骤详情视图组件 ----

function StepDetailView({
  sectionKey,
  stepKey,
  isCompleted,
  onToggle,
  onBack,
  onPrev,
  onNext,
}: {
  sectionKey: string;
  stepKey: string;
  isCompleted: boolean;
  onToggle: () => void;
  onBack: () => void;
  onPrev: (() => void) | null;
  onNext: (() => void) | null;
}) {
  const { t } = useTranslation();
  const i18nBase = `guide.${sectionKey}.${stepKey}`;
  const title = t(`${i18nBase}.title`);
  const desc = t(`${i18nBase}.desc`);
  const sectionTitle = t(`guide.${sectionKey}.title`);

  // detail 字段（Markdown 格式）
  const detailKey = `${i18nBase}.detail`;
  const detailRaw = t(detailKey);
  const hasDetail = detailRaw !== detailKey;

  // tip 字段
  const tipKey = `${i18nBase}.tip`;
  const tip = t(tipKey);
  const hasTip = tip !== tipKey;

  // 构建最终渲染内容
  const markdownContent = useMemo(() => {
    if (hasDetail) {
      // 有 detail 时，用 detail 作为主内容，tip 追加到末尾
      let content = detailRaw;
      if (hasTip) {
        content += `\n\n> **💡 ${t('guide.tip')}：** ${tip}`;
      }
      return content;
    }
    // 没有 detail 时，fallback 显示 desc + tip
    let content = desc;
    if (hasTip) {
      content += `\n\n> **💡 ${t('guide.tip')}：** ${tip}`;
    }
    return content;
  }, [hasDetail, detailRaw, hasTip, tip, desc, t]);

  return (
    <div className="bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl shadow-sm overflow-hidden">
      {/* 面包屑导航 */}
      <div className="px-5 py-3 border-b border-[var(--border-color)] flex items-center gap-2 text-sm">
        <button
          onClick={onBack}
          className="flex items-center gap-1 text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
        >
          <ArrowLeft className="w-4 h-4" />
          <span>{t('guide.back')}</span>
        </button>
        <span className="text-[var(--text-secondary)]">/</span>
        <span className="text-[var(--text-secondary)]">{sectionTitle}</span>
        <span className="text-[var(--text-secondary)]">/</span>
        <span className="text-[var(--text-primary)] font-medium truncate">{stepKey}</span>
      </div>

      {/* 标题 + 完成按钮 */}
      <div className="px-5 py-4 border-b border-[var(--border-color)] flex items-center justify-between gap-4">
        <h2 className="text-lg font-semibold text-[var(--text-primary)] font-display">{title}</h2>
        <button
          onClick={onToggle}
          className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors shrink-0 ${
            isCompleted
              ? 'bg-emerald-100 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-200 dark:hover:bg-emerald-900/30'
              : 'bg-[var(--bg-secondary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)]'
          }`}
        >
          {isCompleted ? (
            <CheckCircle2 className="w-3.5 h-3.5" />
          ) : (
            <Circle className="w-3.5 h-3.5" />
          )}
          <span>{isCompleted ? t('guide.markIncomplete') : t('guide.markComplete')}</span>
        </button>
      </div>

      {/* Markdown 渲染区 */}
      <div className="px-5 py-6">
        <div className="prose prose-sm max-w-none dark:prose-invert">
          <Streamdown>
            {markdownContent}
          </Streamdown>
        </div>
      </div>

      {/* 上一步/下一步导航 */}
      <div className="px-5 py-4 border-t border-[var(--border-color)] flex items-center justify-between">
        {onPrev ? (
          <button
            onClick={onPrev}
            className="flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-lg text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-secondary)] transition-colors"
          >
            <ChevronLeft className="w-4 h-4" />
            <span>{t('guide.prevStep')}</span>
          </button>
        ) : (
          <div />
        )}
        {onNext ? (
          <button
            onClick={onNext}
            className="flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-lg bg-[var(--accent-600)] hover:bg-[var(--accent-700)] text-white transition-colors"
          >
            <span>{t('guide.nextStep')}</span>
            <ChevronRight className="w-4 h-4" />
          </button>
        ) : (
          <div />
        )}
      </div>
    </div>
  );
}

// ---- 步骤组件 ----

function GuideStep({
  sectionKey,
  stepKey,
  isCompleted,
  onToggle,
  onSelect,
}: {
  sectionKey: string;
  stepKey: string;
  isCompleted: boolean;
  onToggle: () => void;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  const i18nBase = `guide.${sectionKey}.${stepKey}`;
  const title = t(`${i18nBase}.title`);
  const desc = t(`${i18nBase}.desc`);
  const tipKey = `${i18nBase}.tip`;
  const tip = t(tipKey);
  const hasTip = tip !== tipKey; // i18next 未找到 key 时返回 key 本身

  return (
    <div
      className={`flex gap-3 p-3 rounded-lg transition-colors cursor-pointer hover:bg-[var(--bg-secondary)] ${
        isCompleted ? 'opacity-75' : ''
      }`}
      onClick={onSelect}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onSelect();
        }
      }}
    >
      <div
        className="pt-0.5 shrink-0"
        onClick={(e) => {
          e.stopPropagation();
          onToggle();
        }}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            e.stopPropagation();
            onToggle();
          }
        }}
      >
        {isCompleted ? (
          <CheckCircle2 className="w-5 h-5 text-emerald-500 hover:text-emerald-600 transition-colors" />
        ) : (
          <Circle className="w-5 h-5 text-[var(--text-secondary)] opacity-40 hover:text-[var(--accent)] transition-colors" />
        )}
      </div>
      <div className="flex-1 min-w-0">
        <h4
          className={`text-sm font-medium ${
            isCompleted
              ? 'line-through text-[var(--text-secondary)]'
              : 'text-[var(--text-primary)]'
          }`}
        >
          {title}
        </h4>
        <p className="text-sm text-[var(--text-secondary)] mt-0.5 whitespace-pre-line line-clamp-2">{desc}</p>
        {hasTip && (
          <div className="flex items-start gap-2 mt-2 p-2.5 bg-[var(--accent-50)] dark:bg-[var(--accent-light)] border border-[var(--accent-border)] rounded-lg">
            <Lightbulb className="w-4 h-4 text-[var(--accent-600)] dark:text-[var(--accent-300)] shrink-0 mt-0.5" />
            <span className="text-xs text-[var(--accent-700)] dark:text-[var(--accent-300)]">{tip}</span>
          </div>
        )}
      </div>
      <div className="shrink-0 flex items-center">
        <ChevronRight className="w-4 h-4 text-[var(--text-secondary)]" />
      </div>
    </div>
  );
}

// ---- Section 卡片组件 ----

function GuideSection({
  section,
  completed,
  onToggleStep,
  onSelectStep,
  sectionRef,
}: {
  section: GuideSectionDef;
  completed: Set<string>;
  onToggleStep: (id: string) => void;
  onSelectStep: (sectionKey: string, stepKey: string) => void;
  sectionRef: (el: HTMLElement | null) => void;
}) {
  const { t } = useTranslation();
  const Icon = section.icon;

  return (
    <section
      ref={sectionRef}
      id={`guide-${section.key}`}
      className="scroll-mt-4 bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl shadow-sm overflow-hidden"
    >
      <div className="px-5 py-4 border-b border-[var(--border-color)]">
        <div className="flex items-center gap-2.5">
          <Icon className="w-5 h-5 text-[var(--accent-600)] dark:text-[var(--accent-300)]" />
          <h2 className="text-lg font-semibold text-[var(--text-primary)] font-display">
            {t(`guide.${section.key}.title`)}
          </h2>
        </div>
        <p className="text-sm text-[var(--text-secondary)] mt-1 ml-7.5">
          {t(`guide.${section.key}.desc`)}
        </p>
      </div>
      <div className="p-3 space-y-1">
        {section.steps.map((step) => {
          const id = stepId(section.key, step.key);
          return (
            <GuideStep
              key={id}
              sectionKey={section.key}
              stepKey={step.key}
              isCompleted={completed.has(id)}
              onToggle={() => onToggleStep(id)}
              onSelect={() => onSelectStep(section.key, step.key)}
            />
          );
        })}
      </div>
    </section>
  );
}

// ---- 页面主组件 ----

export function Guide() {
  const { t } = useTranslation();

  // 进度状态，从 localStorage 恢复
  const [completed, setCompleted] = useState<Set<string>>(() => {
    try {
      const saved = localStorage.getItem('guide-progress');
      return saved ? new Set(JSON.parse(saved) as string[]) : new Set();
    } catch {
      return new Set();
    }
  });

  // 当前选中的步骤（null = 列表视图）
  const [selectedStep, setSelectedStep] = useState<{ sectionKey: string; stepKey: string } | null>(null);

  // 当前可见的 section（scroll-spy）
  const [activeSectionKey, setActiveSectionKey] = useState(GUIDE_SECTIONS[0].key);

  // section 元素引用
  const sectionRefs = useRef<Record<string, HTMLElement | null>>({});

  // 基于滚动容器的 scroll-spy（仅列表视图时生效）
  useEffect(() => {
    if (selectedStep) return;

    // 找到最近的可滚动父容器（AppShell 的 <main class="overflow-auto">）
    const findScrollParent = (el: HTMLElement | null): HTMLElement | null => {
      while (el) {
        const style = getComputedStyle(el);
        if (style.overflow === 'auto' || style.overflowY === 'auto' ||
            style.overflow === 'scroll' || style.overflowY === 'scroll') {
          return el;
        }
        el = el.parentElement;
      }
      return null;
    };

    // 从任意 section ref 向上查找滚动容器
    const firstSection = Object.values(sectionRefs.current).find(Boolean);
    const scrollContainer = findScrollParent(firstSection ?? null);
    if (!scrollContainer) return;

    const handleScroll = () => {
      const containerTop = scrollContainer.getBoundingClientRect().top;
      // 检测点：滚动容器顶部往下 100px 的位置
      const checkPoint = containerTop + 100;

      let activeKey = GUIDE_SECTIONS[0].key;
      for (const section of GUIDE_SECTIONS) {
        const el = sectionRefs.current[section.key];
        if (!el) continue;
        const rect = el.getBoundingClientRect();
        // 如果 section 的顶部在检测点之上，说明已经滚动到或超过了这个 section
        if (rect.top <= checkPoint) {
          activeKey = section.key;
        }
      }
      setActiveSectionKey(activeKey);
    };

    scrollContainer.addEventListener('scroll', handleScroll, { passive: true });
    // 初始化时也执行一次
    handleScroll();

    return () => scrollContainer.removeEventListener('scroll', handleScroll);
  }, [selectedStep]);

  // 切换步骤完成状态
  const toggleStep = useCallback((id: string) => {
    setCompleted((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      localStorage.setItem('guide-progress', JSON.stringify([...next]));
      return next;
    });
  }, []);

  // 重置进度
  const handleReset = useCallback(() => {
    if (window.confirm(t('guide.resetConfirm'))) {
      setCompleted(new Set());
      localStorage.removeItem('guide-progress');
    }
  }, [t]);

  // 导航到 section（列表视图时滚动，详情视图时返回列表并滚动）
  const navigateToSection = useCallback((sectionKey: string) => {
    if (selectedStep) {
      setSelectedStep(null);
      // 等 DOM 更新后滚动
      requestAnimationFrame(() => {
        const el = sectionRefs.current[sectionKey];
        if (el) {
          el.scrollIntoView({ behavior: 'smooth' });
        }
      });
    } else {
      const el = sectionRefs.current[sectionKey];
      if (el) {
        el.scrollIntoView({ behavior: 'smooth' });
      }
    }
    setActiveSectionKey(sectionKey);
  }, [selectedStep]);

  // 选择步骤，进入详情视图
  const selectStep = useCallback((sectionKey: string, stepKey: string) => {
    setSelectedStep({ sectionKey, stepKey });
    setActiveSectionKey(sectionKey);
  }, []);

  // 获取当前步骤在扁平列表中的索引
  const currentFlatIndex = useMemo(() => {
    if (!selectedStep) return -1;
    return FLAT_STEPS.findIndex(
      (s) => s.sectionKey === selectedStep.sectionKey && s.stepKey === selectedStep.stepKey
    );
  }, [selectedStep]);

  // 上一步
  const prevStep = useMemo(() => {
    if (currentFlatIndex <= 0) return null;
    const prev = FLAT_STEPS[currentFlatIndex - 1];
    return () => {
      setSelectedStep({ sectionKey: prev.sectionKey, stepKey: prev.stepKey });
      setActiveSectionKey(prev.sectionKey);
    };
  }, [currentFlatIndex]);

  // 下一步
  const nextStep = useMemo(() => {
    if (currentFlatIndex < 0 || currentFlatIndex >= FLAT_STEPS.length - 1) return null;
    const next = FLAT_STEPS[currentFlatIndex + 1];
    return () => {
      setSelectedStep({ sectionKey: next.sectionKey, stepKey: next.stepKey });
      setActiveSectionKey(next.sectionKey);
    };
  }, [currentFlatIndex]);

  return (
    <div className="p-6 max-w-6xl mx-auto">
      {/* 页面标题 */}
      <div className="flex items-center gap-3 mb-6">
        <BookOpen className="w-6 h-6 text-[var(--accent-600)] dark:text-[var(--accent-300)]" />
        <h1 className="text-xl font-bold text-[var(--text-primary)]">{t('guide.title')}</h1>
      </div>

      {/* 进度条 */}
      <GuideProgressBar done={completed.size} total={TOTAL_STEPS} onReset={handleReset} />

      {/* 主体：目录 + 内容 */}
      <div className="flex gap-6">
        {/* 目录导航 */}
        <GuideTOC
          sections={GUIDE_SECTIONS}
          completed={completed}
          activeSectionKey={activeSectionKey}
          onNavigate={navigateToSection}
        />

        {/* 教程内容 */}
        <div className="flex-1 min-w-0 space-y-6">
          {selectedStep ? (
            <StepDetailView
              sectionKey={selectedStep.sectionKey}
              stepKey={selectedStep.stepKey}
              isCompleted={completed.has(stepId(selectedStep.sectionKey, selectedStep.stepKey))}
              onToggle={() => toggleStep(stepId(selectedStep.sectionKey, selectedStep.stepKey))}
              onBack={() => setSelectedStep(null)}
              onPrev={prevStep}
              onNext={nextStep}
            />
          ) : (
            GUIDE_SECTIONS.map((section) => (
              <GuideSection
                key={section.key}
                section={section}
                completed={completed}
                onToggleStep={toggleStep}
                onSelectStep={selectStep}
                sectionRef={(el) => {
                  sectionRefs.current[section.key] = el;
                }}
              />
            ))
          )}
        </div>
      </div>
    </div>
  );
}
