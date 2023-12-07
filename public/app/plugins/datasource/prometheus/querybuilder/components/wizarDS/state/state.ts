import { createSlice, PayloadAction } from '@reduxjs/toolkit';

import { Interaction, Suggestion, SuggestionType } from '../types';

export const stateSlice = createSlice({
  name: 'wizards-state',
  initialState: initialState([]),
  reducers: {
    showExplainer: (state, action: PayloadAction<boolean>) => {
      state.showExplainer = action.payload;
    },
    showStartingMessage: (state, action: PayloadAction<boolean>) => {
      state.showStartingMessage = action.payload;
    },
    indicateCheckbox: (state, action: PayloadAction<boolean>) => {
      state.indicateCheckbox = action.payload;
    },
    askForHelp: (state, action: PayloadAction<boolean>) => {
      state.askForHelp = action.payload;
    },
    updateTutorialSteps: (state, action: PayloadAction<Suggestion[]>) => {
      const tutorial = state.tutorial;
      const id = tutorial.id + Math.random();
      state.tutorial = { ...tutorial, id, steps: action.payload };
    },
    addInteraction: (state, action: PayloadAction<{ suggestionType: SuggestionType; isLoading: boolean }>) => {
      // AI or Historical?
      const interaction = createInteraction(action.payload.suggestionType, action.payload.isLoading);
      const interactions = state.interactions;
      state.interactions = interactions.concat([interaction]);
    },
    updateInteraction: (state, action: PayloadAction<{ idx: number; interaction: Interaction }>) => {
      // update the interaction by index
      // will most likely be the last interaction but we might update previous by giving them cues of helpful or not
      const index = action.payload.idx;
      const updInteraction = action.payload.interaction;

      state.interactions = state.interactions.map((interaction: Interaction, idx: number) => {
        if (idx === index) {
          return updInteraction;
        }

        return interaction;
      });
    },
  },
});

/**
 * Initial state for wizarDS
 * @param query the prometheus query with metric and possible labels
 */
export function initialState(templates: Suggestion[], showStartingMessage?: boolean): WizarDSState {
  return {
    showExplainer: false,
    showStartingMessage: showStartingMessage ?? true,
    indicateCheckbox: false,
    askForHelp: false,
    interactions: [createInteraction(SuggestionType.AI)],
    tutorial: {
      id: 'wizard-prometheus-version-',
      name: 'Using the PrometheusDS',
      description: `This is a tutorial to help you get started with the PrometheusDS`,
      author: 'LLM-app',
      steps: templates,
    },
  };
}

/**
 * The WizarDS state object
 */
export interface WizarDSState {
  showExplainer: boolean;
  showStartingMessage: boolean;
  indicateCheckbox: boolean;
  askForHelp: boolean;
  interactions: Interaction[];
  tutorial: Tutorial;
}

export type Tutorial = {
  id: string; //'using-prometheusds',
  name: string; //'Using the PrometheusDS',
  description: string; // `This is a tutorial to help you get started with the PrometheusDS`,
  author: string;
  steps: Suggestion[];
};

export function createInteraction(suggestionType: SuggestionType, isLoading?: boolean): Interaction {
  return {
    suggestionType: suggestionType,
    prompt: '',
    suggestions: [],
    isLoading: isLoading ?? false,
    explanationIsLoading: false,
  };
}
