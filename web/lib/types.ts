export interface User {
  id: string;
  email: string;
  name: string;
  role: "student" | "instructor" | "admin";
  leaderboardOptIn: boolean;
}

export interface SkillScore {
  skill: string;
  branch: string;
  mastery: number;
  correctness: number;
  independence: number;
  retention: number;
  difficulty: number;
  incidentPerformance: number;
  attempts: number;
  masteryBadge: boolean;
}

export interface Readiness {
  cert: string;
  name: string;
  percent: number;
  coverage: number;
  explanation: string;
}

export interface GenSpec {
  system: string;
  failures?: string[];
  symptom?: string;
  context?: string;
  difficulty: number;
  mode?: string;
  seed?: number;
  focus?: string[];
}

export interface Recommendation {
  labId?: string;
  title: string;
  reason: string;
  kind: string;
  skill?: string;
  gen?: GenSpec;
}

export interface DimensionScore {
  dimension: string;
  score: number;
  evidence: number;
}

export interface Profile {
  user: User;
  xp: number;
  bonusXp: number;
  level: string;
  levelIndex: number;
  levelProgress: number;
  streak: number;
  skills: SkillScore[] | null;
  branches: Record<string, number>;
  readiness: Readiness[];
  badges: { id: string; name: string; kind: string; reason: string }[] | null;
  recommendations: Recommendation[] | null;
  labsPassed: number;
  attempts: number;
  league: string;
  student: {
    dimensions: DimensionScore[];
    retention: { skill: string; demonstrated: number; retention: number; due: boolean; dueAt: string }[] | null;
    autonomy: { stage: string; index: number; next?: string; progress: number; why: string };
    recurringErrors: { pattern: string; count: number; labs: string[] }[] | null;
    weakestDimensions: string[] | null;
  };
  career: {
    stage: string;
    stageIndex: number;
    next?: string;
    nextNeeds?: string[];
    specializations: { id: string; name: string; unlocked: boolean; percent: number; done: boolean; missing?: string[] }[] | null;
  };
  adaptive: Recommendation[] | null;
}

export interface LabSummary {
  id: string;
  title: string;
  summary: string;
  track: string;
  day: number;
  level: string;
  branch: string;
  type: string;
  skills: string[];
  certs: string[];
  difficulty: number;
  minutes: number;
  fidelity: string[];
  defaultFidelity: string;
  hints: number;
}

export interface Question {
  id: string;
  question: string;
  options: string[];
  justify: boolean;
  after?: string;
  when?: string;
}

export interface SessionInfo {
  id: string;
  labId: string;
  project: string;
  region: string;
  zone: string;
  fidelity: string;
  fidelityReason: string;
  title: string;
  story: string;
  instructions: string;
  objectives: string[] | null;
  timeline: { at: string; from: string; text: string }[] | null;
  constraints: string[] | null;
  hintCount: number;
  evidence?: { prompt: string; fields: string[]; minWords: number };
  quiz?: Question[];
  expires: string;
  simTime: string;
  mode?: string;
  type?: string;
  status: string;
}
